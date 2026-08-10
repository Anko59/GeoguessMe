-- 015_websocket_ticket_auth_version.sql
-- Binds one-time WebSocket tickets to the issuing user's auth_version so a
-- ticket minted before a password change, logout-all, or account revocation
-- can never be consumed afterwards. Forward-only and idempotent under
-- advisory-locked migration execution.

ALTER TABLE websocket_tickets ADD COLUMN IF NOT EXISTS auth_version INTEGER NOT NULL DEFAULT 0;
-- Existing tickets predate the auth-version binding. They are short-lived,
-- but retaining them would let a ticket issued before this migration inherit
-- version zero and potentially survive a concurrent credential revocation.
DELETE FROM websocket_tickets;
CREATE INDEX IF NOT EXISTS websocket_tickets_user_idx ON websocket_tickets(user_id);
