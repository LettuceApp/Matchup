-- +goose Up
-- Most-played matchups — top 9 matchups by raw vote count received in the
-- past hour, excluding owner self-votes. Refreshed hourly by the scheduler
-- (api/scheduler/scheduler.go::jobRefreshTrendingSnapshot).
--
-- Uses the same column set as popular_matchups_snapshot so the Go handler
-- can reuse popularMatchupRow / PopularMatchupDTO without new types.

CREATE MATERIALIZED VIEW public.most_played_snapshot AS
WITH hourly_votes AS (
    SELECT m.id AS matchup_id, COUNT(*) AS votes
    FROM public.matchup_votes mv
    JOIN public.matchups m ON m.public_id = mv.matchup_public_id
    WHERE mv.created_at >= NOW() - INTERVAL '1 hour'
      AND (mv.user_id IS NULL OR mv.user_id <> m.author_id)
    GROUP BY m.id
),
with_meta AS (
    SELECT
        m.id,
        m.title,
        m.author_id,
        m.bracket_id,
        b.author_id AS bracket_author_id,
        m.round,
        b.current_round,
        COALESCE(hv.votes, 0)::bigint AS votes,
        0::bigint AS likes,
        0::bigint AS comments,
        COALESCE(hv.votes, 0)::float8 AS engagement_score,
        ROW_NUMBER() OVER (ORDER BY COALESCE(hv.votes, 0) DESC, m.id ASC)::bigint AS rank
    FROM public.matchups m
    LEFT JOIN public.brackets b  ON b.id = m.bracket_id
    LEFT JOIN hourly_votes   hv ON hv.matchup_id = m.id
    WHERE m.status IN ('active', 'published')
      AND COALESCE(hv.votes, 0) > 0
)
SELECT id, title, author_id, bracket_id, bracket_author_id,
       round, current_round, votes, likes, comments,
       engagement_score, rank
FROM with_meta
ORDER BY rank ASC
LIMIT 9;

CREATE UNIQUE INDEX idx_most_played_snapshot_id
    ON public.most_played_snapshot (id);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS public.most_played_snapshot;
