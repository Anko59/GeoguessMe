-- Reports remain available for moderation until the applicable retention policy
-- removes them. Deleted source content loses its foreign key but retains the
-- contemporaneous notice details and immutable target identifiers.
CREATE TABLE IF NOT EXISTS content_reports (
    id TEXT PRIMARY KEY,
    reporter_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reported_user_id TEXT NOT NULL,
    message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
    photo_id TEXT REFERENCES photos(id) ON DELETE SET NULL,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('user', 'message')),
    target_id TEXT NOT NULL,
    reason TEXT NOT NULL CHECK (reason IN ('illegal_content', 'harassment', 'sexual_content', 'other')),
    details TEXT NOT NULL CHECK (char_length(details) <= 2000),
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed', 'actioned', 'dismissed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    CONSTRAINT content_reports_target_check CHECK (
        (target_kind = 'user' AND message_id IS NULL AND photo_id IS NULL)
        OR (target_kind = 'message' AND photo_id IS NULL)
    ),
    UNIQUE (reporter_id, target_kind, target_id)
);
CREATE INDEX IF NOT EXISTS content_reports_queue_idx ON content_reports (created_at, id) WHERE status = 'open';
