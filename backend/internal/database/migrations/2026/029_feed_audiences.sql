-- Feed audiences are explicit. Public posts remain visible to every signed-in
-- user; friends posts are visible to shared-group members only.
ALTER TABLE public_challenges
    ADD COLUMN IF NOT EXISTS audience TEXT NOT NULL DEFAULT 'public';

DO $$
BEGIN
    ALTER TABLE public_challenges
        ADD CONSTRAINT public_challenges_audience_check
        CHECK (audience IN ('public', 'friends'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

CREATE TABLE IF NOT EXISTS public_challenge_groups (
    challenge_id TEXT NOT NULL REFERENCES public_challenges(id) ON DELETE CASCADE,
    group_id TEXT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    PRIMARY KEY (challenge_id, group_id)
);
CREATE INDEX IF NOT EXISTS public_challenge_groups_group_idx
    ON public_challenge_groups(group_id, challenge_id);
