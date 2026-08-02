-- The challenge viewing window starts when the media has actually been
-- delivered, so slow connections get the full viewing duration instead of
-- the download consuming it. media_delivered_at records the one-time full
-- delivery; view_expires_at is extended to delivery + window on that first
-- delivery and never extended again, preserving the view-once guarantee.
ALTER TABLE challenge_views ADD COLUMN IF NOT EXISTS media_delivered_at TIMESTAMPTZ;

-- Existing view rows predate delivery tracking and may already have consumed
-- their one allowed view. Mark them delivered at acceptance so deploying this
-- migration cannot reopen old challenge media.
UPDATE challenge_views
SET media_delivered_at = accepted_at
WHERE media_delivered_at IS NULL;
