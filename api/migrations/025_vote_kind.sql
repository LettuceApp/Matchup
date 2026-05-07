-- +goose Up
-- Skip / Can't-decide votes.
--
-- A "skip" is a recorded intent to pass on a matchup without picking
-- either contender. Pairwise-comparison research (OpinionX, User
-- Research Strategist case studies) is consistent that forcing a
-- choice on close pairs makes users abandon the flow — a Skip
-- affordance both keeps the user moving and gives bracket owners a
-- signal that a particular pairing is too close.
--
-- Adding a `kind` column to matchup_votes (rather than a separate
-- skips table) lets us reuse identity tracking, the existing
-- switch-vote logic, the Redis vote buffer, and the per-matchup
-- "have they already voted?" lookup. Existing rows backfill to 'pick'
-- via the column default, so this migration is non-breaking.
--
-- Skip rows have:
--   kind = 'skip'
--   matchup_item_public_id = NULL  (skips don't reference an item)
--
-- Pick rows keep their existing semantics. The check constraint on
-- (kind, matchup_item_public_id) below enforces that picks always
-- reference an item and skips never do.

ALTER TABLE public.matchup_votes
    ADD COLUMN kind varchar(8) NOT NULL DEFAULT 'pick';

ALTER TABLE public.matchup_votes
    ADD CONSTRAINT matchup_votes_kind_check
    CHECK (kind IN ('pick', 'skip'));

ALTER TABLE public.matchup_votes
    ALTER COLUMN matchup_item_public_id DROP NOT NULL;

ALTER TABLE public.matchup_votes
    ADD CONSTRAINT matchup_votes_kind_item_check
    CHECK (
        (kind = 'pick' AND matchup_item_public_id IS NOT NULL)
        OR (kind = 'skip' AND matchup_item_public_id IS NULL)
    );

-- Partial index for future skip-tally queries ("what % of votes on
-- this matchup were skips?"). Pick-side queries already have their
-- item-scoped index from the baseline migration. Partial index keeps
-- it tiny — most rows will be picks for the foreseeable future.
CREATE INDEX IF NOT EXISTS idx_matchup_votes_skip_by_matchup
    ON public.matchup_votes (matchup_public_id)
    WHERE kind = 'skip';

-- +goose Down
-- Down migration is destructive: skip rows have no item reference, so
-- there's no way to coerce them back into pick rows once the column
-- nullability tightens. Deleting them is the only path. Re-running
-- Up after a Down would restart from zero skip data — not great but
-- acceptable since skips are recoverable signal, not user-owned content.
DROP INDEX IF EXISTS public.idx_matchup_votes_skip_by_matchup;
DELETE FROM public.matchup_votes WHERE kind = 'skip';
ALTER TABLE public.matchup_votes
    DROP CONSTRAINT IF EXISTS matchup_votes_kind_item_check;
ALTER TABLE public.matchup_votes
    ALTER COLUMN matchup_item_public_id SET NOT NULL;
ALTER TABLE public.matchup_votes
    DROP CONSTRAINT IF EXISTS matchup_votes_kind_check;
ALTER TABLE public.matchup_votes
    DROP COLUMN IF EXISTS kind;
