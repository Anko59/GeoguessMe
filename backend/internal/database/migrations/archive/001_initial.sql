CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    avatar TEXT NOT NULL DEFAULT 'avatar.png',
    score INTEGER NOT NULL DEFAULT 0,
    email TEXT,
    email_normalized TEXT,
    email_verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_normalized TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;
UPDATE users SET email = COALESCE(NULLIF(email, ''), username || '@legacy.invalid') WHERE email IS NULL OR email = '';
UPDATE users SET email_normalized = lower(trim(email)) WHERE email_normalized IS NULL;
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
ALTER TABLE users ALTER COLUMN email_normalized SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS users_email_normalized_key ON users(email_normalized);
ALTER TABLE users DROP COLUMN IF EXISTS score;

CREATE TABLE IF NOT EXISTS groups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    code TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS group_members (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, user_id)
);

CREATE TABLE IF NOT EXISTS photos (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    url TEXT,
    storage_key TEXT,
    mime_type TEXT NOT NULL DEFAULT 'image/jpeg',
    byte_size BIGINT NOT NULL DEFAULT 0,
    lat DOUBLE PRECISION NOT NULL CHECK (lat >= -90 AND lat <= 90),
    long DOUBLE PRECISION NOT NULL CHECK (long >= -180 AND long <= 180),
    lifecycle_status TEXT NOT NULL DEFAULT 'ready' CHECK (lifecycle_status IN ('ready', 'removed', 'deleting')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ NOT NULL,
    retention_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE photos ADD COLUMN IF NOT EXISTS storage_key TEXT;
ALTER TABLE photos ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT 'image/jpeg';
ALTER TABLE photos ADD COLUMN IF NOT EXISTS byte_size BIGINT NOT NULL DEFAULT 0;
ALTER TABLE photos ADD COLUMN IF NOT EXISTS lifecycle_status TEXT NOT NULL DEFAULT 'ready';
ALTER TABLE photos ADD COLUMN IF NOT EXISTS retention_at TIMESTAMPTZ;
UPDATE photos SET storage_key = COALESCE(NULLIF(storage_key, ''), regexp_replace(COALESCE(url, ''), '^/uploads/', '')) WHERE storage_key IS NULL OR storage_key = '';
UPDATE photos SET retention_at = COALESCE(retention_at, COALESCE(expires_at, created_at + interval '30 days')) WHERE retention_at IS NULL;
UPDATE photos SET expires_at = COALESCE(expires_at, created_at + interval '24 hours') WHERE expires_at IS NULL;
ALTER TABLE photos ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE photos ALTER COLUMN retention_at SET NOT NULL;
CREATE INDEX IF NOT EXISTS photos_retention_idx ON photos(retention_at) WHERE lifecycle_status <> 'removed';
CREATE INDEX IF NOT EXISTS photos_expiry_idx ON photos(expires_at);

CREATE TABLE IF NOT EXISTS guesses (
    id TEXT PRIMARY KEY,
    photo_id TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    lat DOUBLE PRECISION NOT NULL CHECK (lat >= -90 AND lat <= 90),
    long DOUBLE PRECISION NOT NULL CHECK (long >= -180 AND long <= 180),
    score INTEGER NOT NULL CHECK (score >= 0),
    distance DOUBLE PRECISION NOT NULL CHECK (distance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(photo_id, user_id)
);
ALTER TABLE guesses ADD COLUMN IF NOT EXISTS group_id TEXT;
UPDATE guesses g SET group_id = p.group_id FROM photos p WHERE g.photo_id = p.id AND g.group_id IS NULL;
ALTER TABLE guesses ALTER COLUMN group_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS guesses_photo_user_key ON guesses(photo_id, user_id);
CREATE INDEX IF NOT EXISTS guesses_group_idx ON guesses(group_id);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'text' CHECK (kind IN ('text', 'challenge', 'system')),
    photo_id TEXT REFERENCES photos(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS kind TEXT NOT NULL DEFAULT 'text';
ALTER TABLE messages ADD COLUMN IF NOT EXISTS photo_id TEXT;
CREATE INDEX IF NOT EXISTS messages_group_created_idx ON messages(group_id, created_at, id);

CREATE TABLE IF NOT EXISTS challenge_views (
    photo_id TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    accepted_at TIMESTAMPTZ NOT NULL,
    view_expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(photo_id, user_id)
);
CREATE INDEX IF NOT EXISTS challenge_views_expiry_idx ON challenge_views(view_expires_at);

CREATE TABLE IF NOT EXISTS refresh_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS refresh_sessions_expiry_idx ON refresh_sessions(expires_at);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS email_verification_expiry_idx ON email_verification_tokens(expires_at);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS password_reset_expiry_idx ON password_reset_tokens(expires_at);

CREATE TABLE IF NOT EXISTS websocket_tickets (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS websocket_tickets_expiry_idx ON websocket_tickets(expires_at);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'photos_user_id_fkey') THEN
        ALTER TABLE photos ADD CONSTRAINT photos_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'photos_group_id_fkey') THEN
        ALTER TABLE photos ADD CONSTRAINT photos_group_id_fkey FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'guesses_photo_id_fkey') THEN
        ALTER TABLE guesses ADD CONSTRAINT guesses_photo_id_fkey FOREIGN KEY (photo_id) REFERENCES photos(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'guesses_user_id_fkey') THEN
        ALTER TABLE guesses ADD CONSTRAINT guesses_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'guesses_group_id_fkey') THEN
        ALTER TABLE guesses ADD CONSTRAINT guesses_group_id_fkey FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'messages_user_id_fkey') THEN
        ALTER TABLE messages ADD CONSTRAINT messages_user_id_fkey FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'messages_group_id_fkey') THEN
        ALTER TABLE messages ADD CONSTRAINT messages_group_id_fkey FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'messages_photo_id_fkey') THEN
        ALTER TABLE messages ADD CONSTRAINT messages_photo_id_fkey FOREIGN KEY (photo_id) REFERENCES photos(id) ON DELETE SET NULL;
    END IF;
END $$;
