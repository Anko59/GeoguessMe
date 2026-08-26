-- Party Time is a recurring, member-started group event window. While a
-- party is active, a member who posts at least one challenge during the
-- window scores double points on their guesses made inside the same window.
--
-- One row records each started party; there is deliberately no "ended" flag.
-- A party is active while started_at <= now < ends_at, the 48h recharge is
-- derived from the latest row's ends_at (next start is allowed at
-- ends_at + PARTY_TIME_COOLDOWN), and old rows are harmless history kept for
-- auditability (at most one row per group per cooldown period).
CREATE TABLE IF NOT EXISTS group_party_times (
    id TEXT PRIMARY KEY,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    started_by TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    started_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    CHECK (ends_at > started_at)
);

-- The start path locks the group row and reads this index for the latest
-- window; the scoring path looks up an active window by group and instant.
CREATE INDEX IF NOT EXISTS group_party_times_group_started_idx
    ON group_party_times(group_id, started_at DESC);
