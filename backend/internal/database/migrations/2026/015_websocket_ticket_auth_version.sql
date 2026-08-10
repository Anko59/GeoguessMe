-- 015_websocket_ticket_auth_version.sql
-- Binds one-time WebSocket tickets to the issuing user's auth_version so a
-- ticket minted before a password change, logout-all, or account revocation
-- can never be consumed afterwards. Forward-only and idempotent under
-- advisory-locked migration execution.

ALTER TABLE websocket_tickets ADD COLUMN IF NOT EXISTS auth_version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS websocket_tickets_user_idx ON websocket_tickets(user_id);
