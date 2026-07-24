package push

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"geoguessme/internal/config"
)

type fakeStore struct {
	mu            sync.Mutex
	targets       []NotificationTarget
	groupName     string
	usernames     map[string]string
	subsByUser    map[string][]Subscription
	deletedIDs    []string
	deleteIDError error
}

func (f *fakeStore) Upsert(_ context.Context, _ *Subscription) error { return nil }
func (f *fakeStore) Delete(_ context.Context, userID, endpoint string) error {
	for i, s := range f.subsByUser[userID] {
		if s.Endpoint == endpoint {
			f.subsByUser[userID] = append(f.subsByUser[userID][:i], f.subsByUser[userID][i+1:]...)
			return nil
		}
	}
	return ErrNoSubscription
}
func (f *fakeStore) ListForUser(_ context.Context, userID string) ([]Subscription, error) {
	return f.subsByUser[userID], nil
}
func (f *fakeStore) ListForUsers(_ context.Context, userIDs []string) ([]Subscription, error) {
	var out []Subscription
	for _, id := range userIDs {
		out = append(out, f.subsByUser[id]...)
	}
	return out, nil
}
func (f *fakeStore) DeleteByID(_ context.Context, id string) error {
	if f.deleteIDError != nil {
		return f.deleteIDError
	}
	f.mu.Lock()
	f.deletedIDs = append(f.deletedIDs, id)
	f.mu.Unlock()
	return nil
}
func (f *fakeStore) GroupTargets(_ context.Context, _, _ string) ([]NotificationTarget, error) {
	return f.targets, nil
}
func (f *fakeStore) GroupName(_ context.Context, _ string) (string, error) {
	if f.groupName == "" {
		return "", ErrNoGroup
	}
	return f.groupName, nil
}
func (f *fakeStore) Username(_ context.Context, userID string) (string, error) {
	if name, ok := f.usernames[userID]; ok {
		return name, nil
	}
	return "", ErrNoUser
}

type fakeDeliverer struct {
	mu      sync.Mutex
	sent    []delivered
	goneFor map[string]bool
	signal  chan struct{}
}

type delivered struct {
	sub     *Subscription
	payload []byte
}

func newFakeDeliverer() *fakeDeliverer {
	return &fakeDeliverer{goneFor: map[string]bool{}, signal: make(chan struct{}, 64)}
}

func (f *fakeDeliverer) Send(_ context.Context, sub *Subscription, payload []byte) error {
	f.mu.Lock()
	f.sent = append(f.sent, delivered{sub: sub, payload: payload})
	gone := f.goneFor[sub.ID]
	f.mu.Unlock()
	f.signal <- struct{}{}
	if gone {
		return fmt.Errorf("%w: simulated gone", ErrSubscriptionGone)
	}
	return nil
}

func (f *fakeDeliverer) snapshot() []delivered {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]delivered, len(f.sent))
	copy(out, f.sent)
	return out
}

func newTestService(store Store, deliver Deliverer) *Service {
	keys, _ := GenerateKeyPair()
	return NewService(Deps{Store: store, Deliver: deliver, Keys: keys, Config: &config.Config{VapidPublicKey: keys.PublicKeyBase64URL(), VapidPrivateKey: keys.PrivateKeyBase64URL()}, Logger: slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))})
}

func waitForSignal(ch <-chan struct{}, timeout time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(timeout):
		return false
	}
}

func TestNotifyNewChallengeDeliversToMembers(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2", Username: "bob"}, {UserID: "u3", Username: "carol"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2", Endpoint: "https://push.example/u2"}},
			"u3": {{ID: "s2", UserID: "u3", Endpoint: "https://push.example/u3"}},
		},
	}
	deliver := newFakeDeliverer()
	svc := newTestService(store, deliver)
	svc.Start(context.Background(), 1)
	defer svc.Stop()

	svc.NotifyNewChallenge(context.Background(), "g1", "u1", "photo-9")
	if !waitForSignal(deliver.signal, time.Second) {
		t.Fatal("expected delivery")
	}
	if !waitForSignal(deliver.signal, time.Second) {
		t.Fatal("expected second delivery")
	}
	sent := deliver.snapshot()
	if len(sent) != 2 {
		t.Fatalf("sent = %d, want 2", len(sent))
	}
	if !containsAll(string(sent[0].payload), "New challenge", "alice", "Paris") {
		t.Fatalf("payload missing fields: %s", sent[0].payload)
	}
}

func TestNotifyNewMessageDeliversWithTag(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2", Username: "bob"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2"}},
		},
	}
	deliver := newFakeDeliverer()
	svc := newTestService(store, deliver)
	svc.Start(context.Background(), 1)
	defer svc.Stop()

	svc.NotifyNewMessage(context.Background(), "g1", "u1", "Anyone playing tonight?")
	if !waitForSignal(deliver.signal, time.Second) {
		t.Fatal("expected one delivery")
	}
	sent := deliver.snapshot()
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sent))
	}
	body := string(sent[0].payload)
	if !containsAll(body, "Paris", "alice", "Anyone playing tonight?", "chat:g1") {
		t.Fatalf("payload missing fields: %s", body)
	}
}

func TestNotifyRemovesGoneSubscription(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "dead", UserID: "u2"}},
		},
	}
	deliver := newFakeDeliverer()
	deliver.goneFor["dead"] = true
	svc := newTestService(store, deliver)
	svc.Start(context.Background(), 1)
	defer svc.Stop()

	svc.NotifyNewMessage(context.Background(), "g1", "u1", "hi")
	if !waitForSignal(deliver.signal, time.Second) {
		t.Fatal("expected delivery attempt")
	}
	// DeleteByID happens after the worker observes the gone error.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		n := len(store.deletedIDs)
		store.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	store.mu.Lock()
	ids := append([]string{}, store.deletedIDs...)
	store.mu.Unlock()
	if len(ids) != 1 || ids[0] != "dead" {
		t.Fatalf("expected dead subscription removed, got %v", store.deletedIDs)
	}
}

func TestNotifySkipsWhenNoTargets(t *testing.T) {
	store := &fakeStore{targets: nil, groupName: "Paris", usernames: map[string]string{"u1": "alice"}}
	deliver := newFakeDeliverer()
	svc := newTestService(store, deliver)
	svc.Start(context.Background(), 1)
	defer svc.Stop()

	svc.NotifyNewChallenge(context.Background(), "g1", "u1", "photo-1")
	// Give the worker a moment to prove it stays idle.
	time.Sleep(50 * time.Millisecond)
	if len(deliver.snapshot()) != 0 {
		t.Fatal("delivery happened with no targets")
	}
}

func TestQueueOverflowDropsWithoutBlocking(t *testing.T) {
	store := &fakeStore{
		targets:   []NotificationTarget{{UserID: "u2"}},
		groupName: "Paris",
		usernames: map[string]string{"u1": "alice"},
		subsByUser: map[string][]Subscription{
			"u2": {{ID: "s1", UserID: "u2"}},
		},
	}
	// Blocking deliverer that never returns: the queue fills and enqueues drop.
	svc := newTestService(store, &blockingDeliverer{})
	ctx, cancel := context.WithCancel(context.Background())
	svc.Start(ctx, 1)
	// Flood far beyond the 256-slot buffer; NotifyNewMessage must return without
	// blocking even though the worker is stuck.
	for i := 0; i < 400; i++ {
		svc.NotifyNewMessage(context.Background(), "g1", "u1", "flood")
	}
	cancel() // unblock the stuck worker so Stop can drain and return
	svc.Stop()
}

type blockingDeliverer struct{}

func (blockingDeliverer) Send(ctx context.Context, _ *Subscription, _ []byte) error {
	<-ctx.Done()
	return ctx.Err()
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		if !strings.Contains(haystack, n) {
			return false
		}
	}
	return true
}
