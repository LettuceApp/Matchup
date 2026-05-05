-- +goose Up
-- Short IDs for shareable links. UUID public IDs are 36 chars which is
-- ugly in URLs meant to go viral. 8-char base62 gives ~218T unique IDs
-- (collision probability under any realistic usage is negligible) while
-- making `matchup.app/m/Kj3mN9xP` look clean.
--
-- Column is nullable at first so existing rows can be backfilled by the
-- api/cmd/backfill_short_ids binary without blocking the migration. A
-- follow-up migration will flip it to NOT NULL once the backfill is done.
--
-- Partial unique index: enforces uniqueness only on non-null values so
-- the backfill can populate rows without fighting a NOT NULL constraint.

ALTER TABLE public.matchups
    ADD COLUMN IF NOT EXISTS short_id TEXT;

ALTER TABLE public.brackets
    ADD COLUMN IF NOT EXISTS short_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_matchups_short_id
    ON public.matchups (short_id)
    WHERE short_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_brackets_short_id
    ON public.brackets (short_id)
    WHERE short_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS public.idx_matchups_short_id;
DROP INDEX IF EXISTS public.idx_brackets_short_id;
ALTER TABLE public.matchups DROP COLUMN IF EXISTS short_id;
ALTER TABLE public.brackets DROP COLUMN IF EXISTS short_id;
