-- Preserve immutable upload metadata so idempotent retries cannot change the
-- captured media or its location/privacy semantics after the first commit.
ALTER TABLE public_challenges
    ADD COLUMN IF NOT EXISTS hide_location BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS byte_size BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS content_digest TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS publication_token TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS public_challenges_publication_token_idx
    ON public_challenges(publication_token)
    WHERE publication_token <> '';
