-- Group invite tokens (F-06). Legacy join codes remain in groups.code for
-- rollback and are no longer accepted by the join contract; this migration
-- only adds the new invite table and its lookup/cap indexes. The groups.code
-- column and its unique index are dropped by a later cleanup migration after
-- the deployment and rollback window close.
CREATE TABLE IF NOT EXISTS group_invites (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    creator_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

-- Active-invite cap lookup per group (revoked_at IS NULL AND expires_at > now).
CREATE INDEX IF NOT EXISTS group_invites_group_idx ON group_invites (group_id);

-- Per-creator daily creation cap lookup.
CREATE INDEX IF NOT EXISTS group_invites_creator_idx ON group_invites (creator_user_id, created_at);
