-- 016 push subscription expiry index.
-- The cleanup worker expires push subscriptions that have not been used for
-- 90 days. The expiry predicate is COALESCE(last_used_at, created_at), so the
-- expression index below keeps the delete cheap as the table grows. It also
-- serves the per-user count queries used by the subscription cap.
CREATE INDEX IF NOT EXISTS push_subscriptions_used_at_idx
    ON push_subscriptions (COALESCE(last_used_at, created_at));
