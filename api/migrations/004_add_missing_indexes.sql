-- +goose Up
-- +goose NO TRANSACTION
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_matchups_author_id ON public.matchups(author_id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_matchups_created_at ON public.matchups(created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_brackets_created_at ON public.brackets(created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_matchup_items_votes ON public.matchup_items(votes DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_matchup_votes_created_at ON public.matchup_votes(created_at DESC);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_comments_matchup_id_created_at ON public.comments(matchup_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_matchups_author_id;
DROP INDEX IF EXISTS idx_matchups_created_at;
DROP INDEX IF EXISTS idx_brackets_created_at;
DROP INDEX IF EXISTS idx_matchup_items_votes;
DROP INDEX IF EXISTS idx_matchup_votes_created_at;
DROP INDEX IF EXISTS idx_comments_matchup_id_created_at;
