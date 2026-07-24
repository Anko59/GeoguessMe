package push

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
	store    Store
	deliver  Deliverer
	keys     *KeyPair
	cfg      *config.Config
	logger   *slog.Logger
	jobs     chan fanoutJob
	wg       sync.WaitGroup
	stopOnce sync.Once
}

type fanoutJob struct {
	userIDs []string
	payload []byte
	reason  string
}

// NewService constructs a notification service. Call Start to launch workers
// and Stop to drain them on shutdown.
func NewService(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Service{
		store:   deps.Store,
		deliver: deps.Deliver,
		keys:    deps.Keys,
		cfg:     deps.Config,
		logger:  deps.Logger,
		jobs:    make(chan fanoutJob, 256),
	}
}

// Keys returns the active VAPID keypair, or nil when push is disabled.
func (s *Service) Keys() *KeyPair { return s.keys }

// Start launches background delivery workers. Workers stop when ctx is cancelled
// and the pending job queue is drained.
func (s *Service) Start(ctx context.Context, workers int) {
	if workers < 1 {
		workers = 2
	}
	for range workers {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

// Stop closes the job queue and waits for in-flight deliveries to finish. It is
// idempotent so callers can invoke it from both explicit shutdown code and a
// deferred safety net. Callers should cancel the run context first so any
// in-progress HTTP send aborts promptly instead of waiting for its timeout.
func (s *Service) Stop() {
	s.stopOnce.Do(func() { close(s.jobs) })
	s.wg.Wait()
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	for job := range s.jobs {
		s.deliverJob(ctx, job)
	}
}

func (s *Service) enqueue(job fanoutJob) {
	select {
	case s.jobs <- job:
	default:
		// A full queue means push delivery is backed up; drop rather than
		// block or grow memory unbounded. Notifications are best-effort.
		s.logger.Warn("push queue full, dropping notification", "reason", job.reason, "recipients", len(job.userIDs))
	}
}

// deliverJob sends the payload to every subscription owned by the target users.
// Permanently invalid subscriptions are removed; transient errors are logged.
func (s *Service) deliverJob(ctx context.Context, job fanoutJob) {
	subs, err := s.store.ListForUsers(ctx, job.userIDs)
	if err != nil {
		s.logger.Error("push target lookup failed", "reason", job.reason, "error", err)
		return
	}
	for i := range subs {
		s.deliverOne(ctx, &subs[i], job.payload)
	}
}

func (s *Service) deliverOne(ctx context.Context, sub *Subscription, payload []byte) {
	sendCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := s.deliver.Send(sendCtx, sub, payload); err != nil {
		if errors.Is(err, ErrSubscriptionGone) {
			if delErr := s.store.DeleteByID(ctx, sub.ID); delErr != nil {
				s.logger.Error("failed to remove invalid push subscription", "subscription_id", sub.ID, "error", delErr)
			} else {
				s.logger.Info("removed invalid push subscription", "subscription_id", sub.ID, "user_id", sub.UserID)
			}
			return
		}
		s.logger.Warn("push delivery failed", "subscription_id", sub.ID, "error", err)
	}
}

// --- handlers.PushNotifier implementation ---------------------------------

// NotifyNewChallenge alerts a group about a freshly uploaded challenge.
func (s *Service) NotifyNewChallenge(ctx context.Context, groupID, excludeUserID, photoID string) {
	targets, groupName, uploader := s.resolveChallenge(ctx, groupID, excludeUserID, photoID)
	if len(targets) == 0 {
		return
	}
	payload := newPayload("New challenge", uploader+" posted a new challenge in "+groupName, groupURL(groupID), "challenge:"+photoID)
	s.enqueue(fanoutJob{userIDs: targetIDs(targets), payload: payload, reason: "new_challenge"})
}

// NotifyNewMessage alerts a group about a new chat message from a member.
func (s *Service) NotifyNewMessage(ctx context.Context, groupID, senderUserID, content string) {
	targets, groupName, sender := s.resolveMessage(ctx, groupID, senderUserID)
	if len(targets) == 0 {
		return
	}
	body := sender + ": " + truncate(strings.TrimSpace(content), 140)
	payload := newPayload(groupName, body, groupURL(groupID), "chat:"+groupID)
	s.enqueue(fanoutJob{userIDs: targetIDs(targets), payload: payload, reason: "new_message"})
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
