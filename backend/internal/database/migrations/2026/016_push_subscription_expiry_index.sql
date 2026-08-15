-- 016 push subscription expiry index.
-- The cleanup worker expires push subscriptions that have not been used for
-- 90 days. The expiry predicate is COALESCE(last_used_at, created_at), so the
-- expression index below keeps the delete cheap as the table grows.
CREATE INDEX IF NOT EXISTS push_subscriptions_used_at_idx
    ON push_subscriptions (COALESCE(last_used_at, created_at));

-- A browser endpoint identifies one concrete PushManager subscription and
-- must belong to only one account. Keep the most recently active copy from
-- legacy data before enforcing global uniqueness; a later subscribe transfers
-- that endpoint atomically to the newly authenticated user.
WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY endpoint
            ORDER BY COALESCE(last_used_at, created_at) DESC, created_at DESC, id ASC
        ) AS endpoint_rank
    FROM push_subscriptions
)

DELETE FROM push_subscriptions AS stale_subscription
USING ranked
WHERE stale_subscription.id = ranked.id AND ranked.endpoint_rank > 1;

CREATE UNIQUE INDEX IF NOT EXISTS push_subscriptions_endpoint_idx
    ON push_subscriptions (endpoint);
