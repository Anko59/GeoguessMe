-- Representative legacy fixture for deterministic migration testing.
-- Models a realistic pre-migration database before any tracked migration
-- has been applied.  Applied directly via psql before the migration runner
-- starts so MigrateUp encounters real legacy rows that exercise every
-- backfill, column-addition, and deduplication path.
--
-- Tables not listed here (challenge_views, refresh_sessions,
-- email_verification_tokens, password_reset_tokens, websocket_tickets,
-- media_deletion_jobs, schema_migrations) did not exist in the legacy
-- schema and are created from scratch by the migrations.

-- ============================================================================
-- users (legacy: no email / auth_version / updated_at / deleted_at columns,
--               has score column that migration 001 drops)
-- ============================================================================
CREATE TABLE users (
    id       TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    avatar   TEXT NOT NULL DEFAULT 'avatar.png',
    score    INTEGER NOT NULL DEFAULT 0
);

INSERT INTO users (id, username, password, avatar, score) VALUES
    -- Normal user – will get email backfill 'player_one@legacy.invalid'
    ('legacy-001', 'player_one',   '$2a$hash1', 'default.png',  42),
    -- Mixed-case username – email_normalized uses lower(trim(email))
    ('legacy-002', 'Player_Two',   '$2a$hash2', 'custom.png',  100),
    -- Username with leading/trailing spaces – exercises trim path
    ('legacy-003', '  spaced_user  ', '$2a$hash3', 'avatar.png',  0),
    -- High-score user – score column is dropped after migration
    ('legacy-004', 'player_four',  '$2a$hash4', 'avatar.png', 9999);

-- ============================================================================
-- groups
-- ============================================================================
CREATE TABLE groups (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    code       TEXT UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO groups (id, name, code, created_at) VALUES
    ('group-001', 'Alpha Squad', 'ALPHA01', '2024-01-10T08:00:00Z'),
    ('group-002', 'Beta Team',   'BETA02',  '2024-02-15T12:00:00Z');

-- ============================================================================
-- group_members
-- ============================================================================
CREATE TABLE group_members (
    group_id  TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (group_id, user_id)
);

INSERT INTO group_members (group_id, user_id, joined_at) VALUES
    ('group-001', 'legacy-001', '2024-01-10T08:05:00Z'),
    ('group-001', 'legacy-002', '2024-01-12T14:30:00Z'),
    ('group-002', 'legacy-003', '2024-02-15T12:10:00Z'),
    ('group-002', 'legacy-004', '2024-03-01T09:00:00Z');

-- ============================================================================
-- photos (legacy: no storage_key, mime_type, byte_size, lifecycle_status,
--         retention_at columns)
-- ============================================================================
CREATE TABLE photos (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id   TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    url        TEXT,
    lat        DOUBLE PRECISION NOT NULL CHECK (lat >= -90  AND lat <= 90),
    long       DOUBLE PRECISION NOT NULL CHECK (long >= -180 AND long <= 180),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ
);

INSERT INTO photos (id, user_id, group_id, url, lat, long, created_at, expires_at) VALUES
    -- has URL → storage_key backfilled from url (strips /uploads/)
    ('photo-001', 'legacy-001', 'group-001',
     '/uploads/abc123.jpg', 48.8566, 2.3522,
     '2024-01-15T10:00:00Z', '2024-01-16T10:00:00Z'),
    -- has URL with different prefix
    ('photo-002', 'legacy-002', 'group-001',
     '/uploads/nested/def456.png', 40.7128, -74.0060,
     '2024-02-20T10:00:00Z', '2024-02-21T10:00:00Z'),
    -- NULL url → storage_key stays NULL, backfill skipped
    ('photo-003', 'legacy-003', 'group-002',
     NULL, 35.6762, 139.6503,
     '2024-03-10T10:00:00Z', '2024-03-11T10:00:00Z'),
    -- NULL expires_at → backfill to created_at + 24h; also NULL retention_at
    ('photo-004', 'legacy-004', 'group-002',
     '/uploads/ghi789.webp', -33.8688, 151.2093,
     '2024-04-01T10:00:00Z', NULL);

-- ============================================================================
-- guesses (legacy: no group_id column)
-- ============================================================================
CREATE TABLE guesses (
    id         TEXT PRIMARY KEY,
    photo_id   TEXT NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lat        DOUBLE PRECISION NOT NULL CHECK (lat >= -90  AND lat <= 90),
    long       DOUBLE PRECISION NOT NULL CHECK (long >= -180 AND long <= 180),
    score      INTEGER NOT NULL CHECK (score >= 0),
    distance   DOUBLE PRECISION NOT NULL CHECK (distance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(photo_id, user_id)
);

INSERT INTO guesses (id, photo_id, user_id, lat, long, score, distance, created_at) VALUES
    ('guess-001', 'photo-001', 'legacy-002',
     48.85, 2.35, 8500, 500.0, '2024-01-15T12:00:00Z'),
    ('guess-002', 'photo-002', 'legacy-001',
     40.71, -74.00, 9200, 200.0, '2024-02-20T15:30:00Z'),
    ('guess-003', 'photo-003', 'legacy-004',
     35.68, 139.65, 7800, 800.0, '2024-03-10T14:00:00Z');

-- ============================================================================
-- messages (legacy: no kind, photo_id columns)
-- ============================================================================
CREATE TABLE messages (
    id         TEXT PRIMARY KEY,
    group_id   TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO messages (id, group_id, user_id, content, created_at) VALUES
    ('msg-001', 'group-001', 'legacy-001', 'Hello team!',        '2024-01-15T12:05:00Z'),
    ('msg-002', 'group-001', 'legacy-002', 'Ready to play?',     '2024-01-15T12:10:00Z'),
    ('msg-003', 'group-002', 'legacy-003', 'Anyone here?',       '2024-03-10T14:05:00Z'),
    ('msg-004', 'group-002', 'legacy-004', 'Nice photo!',        '2024-03-10T14:30:00Z');
