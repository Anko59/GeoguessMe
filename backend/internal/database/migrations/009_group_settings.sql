-- Group photos are private media: the storage key is never exposed to clients
-- and access is checked through group membership at the HTTP boundary.
CREATE TABLE IF NOT EXISTS group_photos (
    group_id TEXT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    storage_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL CHECK (mime_type = 'image/jpeg'),
    byte_size BIGINT NOT NULL CHECK (byte_size > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- An absent preference deliberately means enabled so existing members keep
-- receiving notifications after this migration is deployed.
CREATE TABLE IF NOT EXISTS group_notification_preferences (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, user_id)
);
