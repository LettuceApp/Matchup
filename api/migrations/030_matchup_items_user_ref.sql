-- +goose Up
-- 030_matchup_items_user_ref.sql — let matchup items reference a
-- user. When user_id is set, the item card on the matchup / bracket
-- represents that user as a "contender": their avatar fills the
-- card, their @username shows in place of (or alongside) the text
-- label, and the votes counter still belongs to the item — same
-- vote-counting logic, just visually tied to a person.
--
-- Why nullable + SET NULL on delete: not every item references a
-- user. Free-text + image items are still the default. If a
-- referenced user deletes their account we don't want to invalidate
-- the matchup; the item's user link is severed and the existing
-- `item` text + image_path stay as a fallback display.
--
-- Index: GIN-free partial btree on user_id WHERE user_id IS NOT
-- NULL keeps "matchups I appear in" queries cheap (the future
-- profile-page "Matchups you're in" tab) without bloating the index
-- with the NULL majority.

ALTER TABLE public.matchup_items
  ADD COLUMN user_id BIGINT NULL REFERENCES public.users(id) ON DELETE SET NULL;

CREATE INDEX idx_matchup_items_user_id
  ON public.matchup_items (user_id)
  WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_matchup_items_user_id;
ALTER TABLE public.matchup_items
  DROP COLUMN IF EXISTS user_id;
