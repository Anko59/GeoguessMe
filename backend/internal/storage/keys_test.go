package storage

import "testing"

func TestKeyPrefixHelpers(t *testing.T) {
	photoKey, err := CanonicalKey("photo", "uuid-1")
	if err != nil || photoKey != "photos/uuid-1" {
		t.Fatalf("photo canonical key = %q, %v", photoKey, err)
	}
	chatKey, err := CanonicalKey("chat", "uuid-2")
	if err != nil || chatKey != "chat-media/uuid-2" {
		t.Fatalf("chat canonical key = %q, %v", chatKey, err)
	}
	if _, err := CanonicalKey("video", "uuid-3"); err == nil {
		t.Fatal("unknown canonical kind accepted")
	}

	q := QuarantineKey("raw-1")
	if !IsQuarantineKey(q) || IsCanonicalKey(q) {
		t.Fatalf("quarantine key %q misclassified", q)
	}
	if IsQuarantineKey("photos/uuid-1") || IsQuarantineKey("chat-media/uuid-2") {
		t.Fatal("canonical keys classified as quarantine")
	}
	if !IsCanonicalKey("photos/uuid-1") || !IsCanonicalKey("chat-media/uuid-2") {
		t.Fatal("canonical keys not recognized")
	}
	// Quarantine keys must never be producible through CanonicalKey, so the
	// namespaces are fully partitioned.
	if q == photoKey || q == chatKey {
		t.Fatalf("quarantine key collides with a canonical key: %q", q)
	}
}
