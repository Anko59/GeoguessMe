package push

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"geoguessme/internal/config"

	"github.com/pashagolub/pgxmock/v4"
)

// newConfiguredService builds a service with an explicit configuration so tests
// can exercise the delivery bounds (F-08). A nil cfg yields the defaults.
func newConfiguredService(store Store, deliver Deliverer, cfg *config.Config) *Service {
	keys, _ := GenerateKeyPair()
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.VapidPublicKey = keys.PublicKeyBase64URL()
	cfg.VapidPrivateKey = keys.PrivateKeyBase64URL()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewService(Deps{Store: store, Deliver: deliver, Keys: keys, Config: cfg, Logger: logger})
}

func TestSuccessfulDeliveryTouchesLastUsedAt(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2", Endpoint: "https://push.example/u2"}},
		},
	}
	deliver := newFakeDeliverer()
	svc := newTestService(store, deliver)
	svc.Start(context.Background(), 1)
	defer svc.Stop()

	svc.NotifyNewMessage(context.Background(), "g1", "u1", "hi")
	if !waitForSignal(deliver.signal, time.Second) {
		t.Fatal("expected delivery")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.touchedIDs)
		store.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	store.mu.Lock()
	touched := append([]string{}, store.touchedIDs...)
	store.mu.Unlock()
	if len(touched) != 1 || touched[0] != "s1" {
		t.Fatalf("touched subscriptions = %v, want [s1]", touched)
	}
}

func TestDeliveryTimeoutBoundsSlowSends(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2", Endpoint: "https://push.example/u2"}},
		},
	}
	deliver := &blockingDeliverer{}
	svc := newConfiguredService(store, deliver, &config.Config{PushDeliveryTimeout: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx, 1)

	svc.NotifyNewMessage(context.Background(), "g1", "u1", "hello")
	// The blocking deliverer only returns when its context (the 50ms send
	// deadline) expires; bounded poll instead of a blind sleep.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.metrics.deliveryFailures.Load() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if svc.metrics.deliveryFailures.Load() == 0 {
		t.Fatal("expected the send to fail when the per-send deadline expires")
	}
	if svc.metrics.deliveries.Load() == 0 {
		t.Fatal("expected the delivery attempt to be recorded")
	}
	cancel()
	svc.Stop()
}

// concurrentDeliverer records the maximum number of simultaneous sends and
// blocks each send until release is closed, making per-host concurrency
// deterministic to assert.
type concurrentDeliverer struct {
	mu        sync.Mutex
	started   chan struct{}
	active    int
	maxActive int
	release   chan struct{}
	finished  chan struct{}
}

func newConcurrentDeliverer() *concurrentDeliverer {
	return &concurrentDeliverer{
		started:  make(chan struct{}, 16),
		release:  make(chan struct{}),
		finished: make(chan struct{}, 64),
	}
}

func (d *concurrentDeliverer) Send(ctx context.Context, _ *Subscription, _ []byte) error {
	d.mu.Lock()
	d.active++
	if d.active > d.maxActive {
		d.maxActive = d.active
	}
	d.mu.Unlock()
	select {
	case d.started <- struct{}{}:
	default:
	}
	select {
	case <-d.release:
	case <-ctx.Done():
		d.mu.Lock()
		d.active--
		d.mu.Unlock()
		return ctx.Err()
	}
	d.mu.Lock()
	d.active--
	d.mu.Unlock()
	select {
	case d.finished <- struct{}{}:
	default:
	}
	return nil
}

func TestPerHostConcurrencyCap(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u1"}, {UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u0": "alice"},
		subsByUser: map[string][]Subscription{
			"u1": {{ID: "s1", UserID: "u1", Endpoint: "https://fcm.googleapis.com/a"}},
			"u2": {{ID: "s2", UserID: "u2", Endpoint: "https://fcm.googleapis.com/b"}},
		},
	}
	deliver := newConcurrentDeliverer()
	// PUSH_DELIVERY_PER_HOST=1 forces sends to the same host to serialize even
	// with two global workers in flight.
	svc := newConfiguredService(store, deliver, &config.Config{PushDeliveryPerHost: 1})
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx, 2)
	defer func() {
		cancel()
		svc.Stop()
	}()

	svc.NotifyNewChallenge(context.Background(), "g1", "u0", "photo-1")
	svc.NotifyNewMessage(context.Background(), "g1", "u0", "hello")
	if !waitForSignal(deliver.started, time.Second) {
		t.Fatal("expected the first send to start")
	}
	deliver.mu.Lock()
	first := deliver.maxActive
	deliver.mu.Unlock()
	if first != 1 {
		t.Fatalf("concurrent sends to one host while blocked = %d, want 1", first)
	}
	close(deliver.release)
	// Drain every send (four deliveries: two jobs x two recipients) and then
	// confirm the cap held for the whole burst.
	for i := 0; i < 4; i++ {
		if !waitForSignal(deliver.finished, 2*time.Second) {
			t.Fatalf("delivery %d did not finish", i)
		}
	}
	deliver.mu.Lock()
	defer deliver.mu.Unlock()
	if deliver.maxActive != 1 {
		t.Fatalf("max concurrent sends to one host = %d, want 1", deliver.maxActive)
	}
}

func TestQueueMetricsRecordDepthAndDrops(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2"}},
		},
	}
	// Blocking deliverer never returns: the queue fills and enqueues drop.
	svc := newTestService(store, &blockingDeliverer{})
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx, 1)
	for i := 0; i < 400; i++ {
		svc.NotifyNewMessage(context.Background(), "g1", "u1", "flood")
	}
	if drops := svc.metrics.drops.Load(); drops == 0 {
		t.Fatal("expected dropped notifications to be counted")
	}
	if depth := svc.metrics.queueDepth.Load(); depth < 0 || depth > 256 {
		t.Fatalf("queue depth gauge out of bounds: %d", depth)
	}
	cancel() // unblock the stuck worker so Stop can drain and return
	svc.Stop()
}

func TestEnqueueAfterStopIsSafeAndAccountedAsDrop(t *testing.T) {
	svc := newTestService(&fakeStore{}, &fakeDeliverer{})
	svc.Stop()

	svc.enqueue(fanoutJob{reason: "shutdown-race"})

	if got := svc.metrics.queueDepth.Load(); got != 0 {
		t.Fatalf("queue depth after stopped enqueue = %d, want 0", got)
	}
	if got := svc.metrics.drops.Load(); got != 1 {
		t.Fatalf("drops after stopped enqueue = %d, want 1", got)
	}
}

func TestEndpointHostNormalizesCaseAndPort(t *testing.T) {
	if got := endpointHost("https://FCM.GoogleApis.Com:8443/send"); got != "fcm.googleapis.com" {
		t.Fatalf("endpointHost = %q, want normalized hostname", got)
	}
}

func TestHostSemaphoreCacheIsBounded(t *testing.T) {
	svc := newConfiguredService(&fakeStore{}, &fakeDeliverer{}, &config.Config{PushDeliveryPerHost: 1})
	var overflow chan struct{}
	for i := 0; i < 64; i++ {
		sem := svc.hostSem(fmt.Sprintf("provider-%d.example", i))
		if i >= 32 {
			if overflow == nil {
				overflow = sem
			} else if sem != overflow {
				t.Fatal("overflow hosts must share the bounded fallback semaphore")
			}
		}
	}
	if got := len(svc.hostSems); got != 32 {
		t.Fatalf("host semaphore cache size = %d, want 32", got)
	}
}

func TestMetricsTextRendersPushMetrics(t *testing.T) {
	svc := newTestService(&fakeStore{}, &fakeDeliverer{})
	svc.metrics.queueDepth.Store(2)
	svc.metrics.drops.Store(7)
	svc.metrics.deliveryFailures.Store(1)
	svc.metrics.subscriptions.Store(42)
	svc.metrics.observeDuration(0.3)

	text := svc.MetricsText()
	for _, want := range []string{
		"push_queue_depth 2",
		"push_drops_total 7",
		"push_delivery_failures_total 1",
		"push_subscriptions_total 42",
		"push_delivery_duration_seconds_count 1",
		`push_delivery_duration_seconds_bucket{le="0.5"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics text missing %q in %q", want, text)
		}
	}
}

func TestSubscriptionCountGaugeRefresh(t *testing.T) {
	store := &fakeStore{subsByUser: map[string][]Subscription{
		"u1": {{ID: "s1", UserID: "u1"}, {ID: "s2", UserID: "u1"}},
	}}
	svc := newTestService(store, &fakeDeliverer{})
	svc.refreshSubscriptionCount(context.Background())
	if got := svc.metrics.subscriptions.Load(); got != 2 {
		t.Fatalf("subscriptions gauge = %d, want 2", got)
	}
}

// --- store-level cap enforcement (pgxmock) --------------------------------

func newStoreMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

func TestUpsertRejectsNewEndpointAtCap(t *testing.T) {
	mock := newStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM users WHERE id = \\$1 FOR UPDATE").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("push_subscriptions WHERE user_id = \\$1 AND endpoint = \\$2").WithArgs("user-1", "https://fcm.example/a").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("push_subscriptions WHERE user_id = \\$1").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectRollback()

	err := (pgStore{pool: mock}).Upsert(context.Background(), &Subscription{UserID: "user-1", Endpoint: "https://fcm.example/a"}, 5)
	if !errors.Is(err, ErrSubscriptionLimit) {
		t.Fatalf("Upsert error = %v, want ErrSubscriptionLimit", err)
	}
}

func TestUpsertRefreshesExistingEndpointAtCap(t *testing.T) {
	mock := newStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM users WHERE id = \\$1 FOR UPDATE").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("push_subscriptions WHERE user_id = \\$1 AND endpoint = \\$2").WithArgs("user-1", "https://fcm.example/a").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO push_subscriptions(?s).*created_at = EXCLUDED.created_at.*last_used_at = NULL").WithArgs(
		pgxmock.AnyArg(), "user-1", "https://fcm.example/a", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	err := (pgStore{pool: mock}).Upsert(context.Background(), &Subscription{UserID: "user-1", Endpoint: "https://fcm.example/a", P256DH: "k", Auth: "s"}, 5)
	if err != nil {
		t.Fatalf("Upsert refresh error = %v", err)
	}
}

func TestUpsertInsertsNewEndpointUnderCap(t *testing.T) {
	mock := newStoreMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT id FROM users WHERE id = \\$1 FOR UPDATE").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("push_subscriptions WHERE user_id = \\$1 AND endpoint = \\$2").WithArgs("user-1", "https://fcm.example/b").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("push_subscriptions WHERE user_id = \\$1").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(3))
	mock.ExpectExec("INSERT INTO push_subscriptions").WithArgs(
		pgxmock.AnyArg(), "user-1", "https://fcm.example/b", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(),
	).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	err := (pgStore{pool: mock}).Upsert(context.Background(), &Subscription{UserID: "user-1", Endpoint: "https://fcm.example/b", P256DH: "k", Auth: "s"}, 5)
	if err != nil {
		t.Fatalf("Upsert insert error = %v", err)
	}
}

func TestCountAndTouchSubscriptions(t *testing.T) {
	mock := newStoreMock(t)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM push_subscriptions WHERE user_id = \\$1").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	count, err := (pgStore{pool: mock}).CountSubscriptionsByUser(context.Background(), "user-1")
	if err != nil || count != 2 {
		t.Fatalf("CountSubscriptionsByUser = %d, %v", count, err)
	}

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM push_subscriptions").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(9))
	total, err := (pgStore{pool: mock}).CountAllSubscriptions(context.Background())
	if err != nil || total != 9 {
		t.Fatalf("CountAllSubscriptions = %d, %v", total, err)
	}

	mock.ExpectExec("UPDATE push_subscriptions SET last_used_at").WithArgs("s1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	if err := (pgStore{pool: mock}).TouchSubscription(context.Background(), "s1"); err != nil {
		t.Fatalf("TouchSubscription error = %v", err)
	}
}
