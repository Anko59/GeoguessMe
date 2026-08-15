package storage

import (
	"fmt"
	"strings"
)

// Media key-prefix helpers. Object keys are the only addressable dimension of
// the private object store: authenticated serving handlers resolve keys
// exclusively from database records (photos.storage_key and
// chat_media.storage_key). Keys under the quarantine prefix are therefore
// unreachable by construction — no served handler ever resolves them, and the
// raw upload is either promoted to a canonical key on success or deleted by
// the cleanup runner on failure. This invariant is asserted by
// TestQuarantineKeysAreUnservable.
const (
	// PhotoKeyPrefix is the canonical key prefix for normalized challenge
	// photos served by ServeChallengeMedia.
	PhotoKeyPrefix = "photos/"
	// ChatMediaKeyPrefix is the canonical key prefix for chat attachments
	// served by ServeChatMedia.
	ChatMediaKeyPrefix = "chat-media/"
	// QuarantineKeyPrefix holds raw asynchronous uploads (videos) while a
	// processing job is outstanding. Nothing ever serves from this prefix.
	QuarantineKeyPrefix = "quarantine/"
)

// QuarantineKey returns the private storage key for a raw asynchronous upload
// identified by the given UUID. The object is never served: it is promoted to
// a canonical key on success or deleted by the cleanup runner on failure.
func QuarantineKey(id string) string {
	return QuarantineKeyPrefix + id
}

// CanonicalKey returns the served storage key for a completed media object.
// kind must be one of the served media kinds ("photo" or "chat"); it maps to
// the same prefixes the upload handlers use for synchronous media.
func CanonicalKey(kind, id string) (string, error) {
	switch kind {
	case "photo":
		return PhotoKeyPrefix + id, nil
	case "chat":
		return ChatMediaKeyPrefix + id, nil
	default:
		return "", fmt.Errorf("unknown canonical media kind %q", kind)
	}
}

// IsQuarantineKey reports whether a storage key lives under the private
// quarantine prefix.
func IsQuarantineKey(key string) bool {
	return strings.HasPrefix(key, QuarantineKeyPrefix)
}

// IsCanonicalKey reports whether a storage key lives under one of the served
// canonical prefixes (photos/ or chat-media/). The media serving handlers
// reject any key that is not canonical as a defense-in-depth guard: even a
// database row that referenced a quarantine or unknown key could never stream
// raw quarantine objects.
func IsCanonicalKey(key string) bool {
	return strings.HasPrefix(key, PhotoKeyPrefix) || strings.HasPrefix(key, ChatMediaKeyPrefix)
}
