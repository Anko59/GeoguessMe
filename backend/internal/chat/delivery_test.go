package chat

import (
	"context"
	"errors"
	"sync"
	"testing"

	"geoguessme/internal/models"
)

func TestQueuedDeliveryReauthorizesAfterBlock(t *testing.T) {
	hub := NewHub(nil, nil)
	blocked := false
	hub.Delivery = func(_ context.Context, queued models.Message, viewer string) (*models.Message, error) {
		if viewer != "viewer" {
			t.Errorf("unexpected viewer %s", viewer)
		}
		if blocked {
			return nil, nil
		}
		queued.Content = "authoritative"
		return &queued, nil
	}
	client := &Client{hub: hub, userID: "viewer"}
	queued := models.Message{ID: "old", Kind: "text", Content: "stale"}
	written := []string{}
	write := func(m models.Message) error { written = append(written, m.Content); return nil }
	if err := client.deliverAuthorized(queued, write); err != nil {
		t.Fatal(err)
	}
	if err := hub.SerializePrivacyChange(func() error { blocked = true; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := client.deliverAuthorized(queued, write); err != nil {
		t.Fatal(err)
	}
	queued.Kind = "system"
	if err := client.deliverAuthorized(queued, write); err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != "authoritative" {
		t.Fatalf("queued leak: %v", written)
	}
	hub.Delivery = func(context.Context, models.Message, string) (*models.Message, error) {
		return nil, errors.New("database unavailable")
	}
	if err := client.deliverAuthorized(queued, write); err != nil || len(written) != 1 {
		t.Fatalf("fail-open: %v %v", written, err)
	}
}

func TestPrivacyCommitWaitsForInFlightDelivery(t *testing.T) {
	hub := NewHub(nil, nil)
	client := &Client{hub: hub}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var mu sync.Mutex
	order := []string{}
	go func() {
		done <- client.deliverAuthorized(models.Message{Kind: "text"}, func(models.Message) error {
			close(entered)
			<-release
			mu.Lock()
			order = append(order, "write")
			mu.Unlock()
			return nil
		})
	}()
	<-entered
	changeStarted, changed := make(chan struct{}), make(chan error, 1)
	go func() {
		close(changeStarted)
		changed <- hub.SerializePrivacyChange(func() error {
			mu.Lock()
			order = append(order, "commit")
			mu.Unlock()
			return nil
		})
	}()
	<-changeStarted
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "write" || order[1] != "commit" {
		t.Fatalf("commit overtook delivery: %v", order)
	}
}
