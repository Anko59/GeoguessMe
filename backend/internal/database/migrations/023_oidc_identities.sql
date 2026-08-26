-- Add Keycloak/OIDC identities beside the existing application accounts.
-- Existing user IDs remain unchanged so memberships, scores, messages, media,
-- and every other user-owned row survive the authentication migration.
ALTER TABLE users
ADD COLUMN IF NOT EXISTS legacy_password_enabled BOOLEAN NOT NULL DEFAULT TRUE;

CREATE TABLE IF NOT EXISTS user_identities (
    issuer TEXT NOT NULL,
    subject TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email_at_link TEXT,
    linked_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (issuer, subject),
    UNIQUE (user_id, issuer),
    CHECK (length(issuer) BETWEEN 1 AND 2048),
    CHECK (length(subject) BETWEEN 1 AND 255)
);

CREATE INDEX IF NOT EXISTS user_identities_user_id_idx
ON user_identities(user_id);

-- A link intent proves that the browser first authenticated with the legacy
-- account before returning from Keycloak. Only a SHA-256 digest is stored;
-- intents are single-use and short-lived.
CREATE TABLE IF NOT EXISTS oidc_link_intents (
    token_hash TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS oidc_link_intents_expiry_idx
ON oidc_link_intents(expires_at);
