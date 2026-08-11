package push

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"geoguessme/internal/config"
)

// Deliverer encrypts and sends one push message. *Sender implements it; tests
// inject a recorder.
type Deliverer interface {
	Send(ctx context.Context, sub *Subscription, payload []byte) error
}

// Deps wires the notification service.
type Deps struct {
	Store   Store
	Deliver Deliverer
	Keys    *KeyPair
	Config  *config.Config
	Logger  *slog.Logger
}

// Service fans push notifications out to subscribers asynchronously so the
// triggering request is never blocked by slow or unreachable push services.
// It implements the handlers.PushNotifier interface structurally.
type Service struct {
	store       Store
	deliver     Deliverer
	keys        *KeyPair
	cfg         *config.Config
	guard       *EndpointGuard
	logger      *slog.Logger
	jobs        chan fanoutJob
	wg          sync.WaitGroup
	stopOnce    sync.Once
	stopCh      chan struct{}
	enqueueMu   sync.RWMutex
	stopping    bool
	metrics     *serviceMetrics
	hostSems    map[string]chan struct{}
	overflowSem chan struct{}
	hostSemsMu  sync.Mutex
}

type fanoutJob struct {
	userIDs []string
	payload []byte
	reason  string
	groupID string
}

// NewService constructs a notification service. Call Start to launch workers
// and Stop to drain them on shutdown.
func NewService(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	svc := &Service{
		store:       deps.Store,
		deliver:     deps.Deliver,
		keys:        deps.Keys,
		cfg:         deps.Config,
		logger:      deps.Logger,
		jobs:        make(chan fanoutJob, queueDepthFromConfig(deps.Config)),
		stopCh:      make(chan struct{}),
		metrics:     newServiceMetrics(),
		hostSems:    make(map[string]chan struct{}),
		overflowSem: make(chan struct{}, deliveryPerHostFromConfig(deps.Config)),
	}
	if deps.Config != nil {
		svc.guard = NewEndpointGuard(deps.Config.PushEndpointAllowlist, deps.Config.Environment != config.EnvProduction)
	}
	return svc
}

// Keys returns the active VAPID keypair, or nil when push is disabled.
func (s *Service) Keys() *KeyPair { return s.keys }

// Start launches background delivery workers and the subscription-count
// refresh loop. Workers stop when ctx is cancelled; Stop accounts for any
// queued best-effort jobs as drops. The explicit workers argument is honored when positive; a
// non-positive value falls back to the configured PUSH_DELIVERY_WORKERS.
func (s *Service) Start(ctx context.Context, workers int) {
	if s.keys == nil {
		return
	}
	if workers < 1 {
		workers = s.deliveryWorkers()
	}
	for range workers {
		s.wg.Add(1)
		go s.worker(ctx)
	}
	s.wg.Add(1)
	go s.subscriptionCountLoop(ctx)
}

// Stop signals background workers and the metrics refresh loop, waiting for
// in-flight deliveries to finish. The jobs channel deliberately remains open:
// request handlers can race with shutdown, and closing it would make an
// otherwise harmless late notification panic. It is idempotent
// so callers can invoke it from both explicit shutdown code and a deferred
// safety net. Callers should cancel the run context first so any in-progress
// HTTP send aborts promptly instead of waiting for its timeout.
func (s *Service) Stop() {
	s.stopOnce.Do(func() {
		s.enqueueMu.Lock()
		s.stopping = true
		close(s.stopCh)
		s.enqueueMu.Unlock()
	})
	s.wg.Wait()
	// No producer can publish after stopping is set. Account for jobs which
	// were accepted before shutdown but not selected by a worker.
	for {
		select {
		case <-s.jobs:
			s.metrics.queueDepth.Add(-1)
			s.metrics.drops.Add(1)
		default:
			return
		}
	}
}

// --- configuration-backed delivery bounds ---------------------------------

func queueDepthFromConfig(cfg *config.Config) int {
	if cfg != nil && cfg.PushQueueDepth > 0 {
		return cfg.PushQueueDepth
	}
	return 256
}

// MaxSubscriptionsPerUser returns the configured per-user subscription cap; the
// subscribe handler uses it for its friendly pre-check while the transactional
// cap inside the store remains authoritative.
func (s *Service) MaxSubscriptionsPerUser() int {
	if s.cfg != nil && s.cfg.PushMaxSubscriptionsPerUser > 0 {
		return s.cfg.PushMaxSubscriptionsPerUser
	}
	return 5
}

func (s *Service) deliveryWorkers() int {
	if s.cfg != nil && s.cfg.PushDeliveryWorkers > 0 {
		return s.cfg.PushDeliveryWorkers
	}
	return 4
}

func (s *Service) deliveryPerHost() int {
	return deliveryPerHostFromConfig(s.cfg)
}

func deliveryPerHostFromConfig(cfg *config.Config) int {
	if cfg != nil && cfg.PushDeliveryPerHost > 0 {
		return cfg.PushDeliveryPerHost
	}
	return 2
}

func (s *Service) deliveryTimeout() time.Duration {
	if s.cfg != nil && s.cfg.PushDeliveryTimeout > 0 {
		return s.cfg.PushDeliveryTimeout
	}
	return 5 * time.Second
}

// hostSem returns the per-host concurrency gate for one push-service host,
// creating it on first use. The gate capacity is PUSH_DELIVERY_PER_HOST.
func (s *Service) hostSem(host string) chan struct{} {
	s.hostSemsMu.Lock()
	defer s.hostSemsMu.Unlock()
	sem, ok := s.hostSems[host]
	if !ok {
		// Provider subdomains are attacker-controlled subscription input. Keep
		// their semaphore cache bounded and share one conservative overflow gate.
		if len(s.hostSems) >= 32 {
			return s.overflowSem
		}
		perHost := s.deliveryPerHost()
		if perHost < 1 {
			perHost = 2
		}
		sem = make(chan struct{}, perHost)
		s.hostSems[host] = sem
	}
	return sem
}

// endpointHost extracts the host portion of a push endpoint for per-host
// concurrency gating. An unparseable endpoint yields an empty host, which skips
// the gate; the sender's allowlist validation rejects such endpoints anyway.
func endpointHost(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

// --- subscription-count gauge ---------------------------------------------

// subscriptionCountRefreshInterval bounds the rate of COUNT queries backing the
// push_subscriptions_total gauge. One minute is far below the churn of a
// subscription table and costs a single cheap count on a single replica.
const subscriptionCountRefreshInterval = time.Minute

func (s *Service) subscriptionCountLoop(ctx context.Context) {
	defer s.wg.Done()
	s.refreshSubscriptionCount(ctx)
	ticker := time.NewTicker(subscriptionCountRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.refreshSubscriptionCount(ctx)
		}
	}
}

func (s *Service) refreshSubscriptionCount(ctx context.Context) {
	count, err := s.store.CountAllSubscriptions(ctx)
	if err != nil {
		s.logger.Warn("push subscription count refresh failed", "error", err)
		return
	}
	s.metrics.subscriptions.Store(count)
}

// --- metrics surface ------------------------------------------------------

// MetricsText renders the push delivery metrics as Prometheus text. The
// composition root wires it into the /metrics endpoint through the middleware
// metrics handler's extra renderer.
func (s *Service) MetricsText() string {
	var builder strings.Builder
	s.metrics.render(&builder)
	return builder.String()
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case job := <-s.jobs:
			s.metrics.queueDepth.Add(-1)
			s.deliverJob(ctx, job)
		}
	}
}

func (s *Service) enqueue(job fanoutJob) {
	s.enqueueMu.RLock()
	defer s.enqueueMu.RUnlock()
	if s.stopping {
		s.metrics.drops.Add(1)
		return
	}
	// Increment before publishing the job. A worker may receive immediately,
	// so incrementing after the send can transiently expose a negative gauge.
	s.metrics.queueDepth.Add(1)
	select {
	case s.jobs <- job:
	default:
		s.metrics.queueDepth.Add(-1)
		// A full queue means push delivery is backed up; drop rather than
		// block or grow memory unbounded. Notifications are best-effort.
		s.metrics.drops.Add(1)
		s.logger.Warn("push queue full, dropping notification", "reason", job.reason, "recipients", len(job.userIDs))
	}
}

// deliverJob sends the payload to every subscription owned by the target users.
// Permanently invalid subscriptions are removed; transient errors are logged.
func (s *Service) deliverJob(ctx context.Context, job fanoutJob) {
	// Recheck membership at delivery time. A member removed after enqueue must
	// never receive delayed group names or message content from the queue.
	subs, err := s.store.ListForGroupUsers(ctx, job.groupID, job.userIDs)
	if err != nil {
		s.logger.Error("push target lookup failed", "reason", job.reason, "error", err)
		return
	}
	for i := range subs {
		s.deliverOne(ctx, &subs[i], job.payload)
	}
}

func (s *Service) deliverOne(ctx context.Context, sub *Subscription, payload []byte) {
	host := endpointHost(sub.Endpoint)
	// Cap concurrent sends to one push-service host (PUSH_DELIVERY_PER_HOST)
	// across the global worker pool. Waiting on the slot honours ctx so a
	// shutdown aborts promptly instead of piling up blocked sends.
	if host != "" {
		sem := s.hostSem(host)
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
		case <-ctx.Done():
			return
		}
	}
	sendCtx, cancel := context.WithTimeout(ctx, s.deliveryTimeout())
	defer cancel()
	started := time.Now()
	err := s.deliver.Send(sendCtx, sub, payload)
	s.metrics.deliveries.Add(1)
	s.metrics.observeDuration(time.Since(started).Seconds())
	if err != nil {
		s.metrics.deliveryFailures.Add(1)
		if errors.Is(err, ErrSubscriptionGone) {
			if delErr := s.store.DeleteByID(ctx, sub.ID); delErr != nil {
				s.logger.Error("failed to remove invalid push subscription", "subscription_id", sub.ID, "error", delErr)
			} else {
				s.logger.Info("removed invalid push subscription", "subscription_id", sub.ID, "user_id", sub.UserID)
			}
			return
		}
		s.logger.Warn("push delivery failed", "subscription_id", sub.ID, "error", err)
		return
	}
	// A successful delivery refreshes last_used_at so the expiry cleanup never
	// drops an actively used subscription. A missing row is tolerated: a
	// concurrent cleanup may have removed it between send and touch.
	if touchErr := s.store.TouchSubscription(ctx, sub.ID); touchErr != nil {
		s.logger.Warn("push subscription touch failed", "subscription_id", sub.ID, "error", touchErr)
	}
}

// --- handlers.PushNotifier implementation ---------------------------------

// NotifyNewChallenge alerts a group about a freshly uploaded challenge.
func (s *Service) NotifyNewChallenge(ctx context.Context, groupID, excludeUserID, photoID string) {
	if s.keys == nil {
		return
	}
	targets, groupName, uploader := s.resolveChallenge(ctx, groupID, excludeUserID, photoID)
	if len(targets) == 0 {
		return
	}
	payload := newPayload("New challenge", uploader+" posted a new challenge in "+groupName, groupURL(groupID), "challenge:"+photoID)
	s.enqueue(fanoutJob{userIDs: targetIDs(targets), payload: payload, reason: "new_challenge", groupID: groupID})
}

// NotifyNewMessage alerts a group about a new chat message from a member.
func (s *Service) NotifyNewMessage(ctx context.Context, groupID, senderUserID, content string) {
	if s.keys == nil {
		return
	}
	targets, groupName, sender := s.resolveMessage(ctx, groupID, senderUserID)
	if len(targets) == 0 {
		return
	}
	body := sender + ": " + truncate(strings.TrimSpace(content), 140)
	payload := newPayload(groupName, body, groupURL(groupID), "chat:"+groupID)
	s.enqueue(fanoutJob{userIDs: targetIDs(targets), payload: payload, reason: "new_message", groupID: groupID})
}

func (s *Service) resolveChallenge(ctx context.Context, groupID, excludeUserID, photoID string) (targets []NotificationTarget, groupName, uploader string) {
	groupName, err := s.store.GroupName(ctx, groupID)
	if err != nil {
		s.logger.Warn("push group name lookup failed", "group_id", groupID, "error", err)
		groupName = "a group"
	}
	targets, err = s.store.GroupTargets(ctx, groupID, excludeUserID)
	if err != nil {
		s.logger.Error("push target lookup failed", "group_id", groupID, "error", err)
		return nil, groupName, "Someone"
	}
	uploader, err = s.store.Username(ctx, excludeUserID)
	if err != nil {
		uploader = "Someone"
	}
	return targets, groupName, uploader
}

func (s *Service) resolveMessage(ctx context.Context, groupID, senderUserID string) (targets []NotificationTarget, groupName, sender string) {
	groupName, err := s.store.GroupName(ctx, groupID)
	if err != nil {
		groupName = "GeoGuessMe"
	}
	sender, err = s.store.Username(ctx, senderUserID)
	if err != nil {
		sender = "Someone"
	}
	targets, err = s.store.GroupTargets(ctx, groupID, senderUserID)
	if err != nil {
		s.logger.Error("push target lookup failed", "group_id", groupID, "error", err)
		return nil, groupName, sender
	}
	return targets, groupName, sender
}

func targetIDs(targets []NotificationTarget) []string {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.UserID)
	}
	return ids
}

// pushPayload is the JSON contract the service worker reads in its push event.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	Badge string `json:"badge,omitempty"`
}

func newPayload(title, body, pageURL, tag string) []byte {
	raw, _ := json.Marshal(pushPayload{Title: title, Body: body, URL: pageURL, Tag: tag})
	return raw
}

func groupURL(groupID string) string { return "/group/" + groupID }

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
