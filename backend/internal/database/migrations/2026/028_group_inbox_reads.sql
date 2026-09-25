-- The feed rail's unread state is authoritative and durable per member.
CREATE TABLE IF NOT EXISTS group_message_reads (
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX IF NOT EXISTS group_message_reads_user_idx
    ON group_message_reads(user_id, group_id);
