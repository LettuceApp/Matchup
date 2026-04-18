-- +goose Up
-- Hourly trending matchups — top 9 matchups by engagement received in the
-- past hour, excluding owner self-interactions. Refreshed on the hour by
-- the scheduler (api/scheduler/scheduler.go::jobRefreshTrendingSnapshot).
--
-- Uses the same column set as popular_matchups_snapshot so the Go handler
-- can reuse popularMatchupRow / PopularMatchupDTO without new types.

CREATE MATERIALIZED VIEW public.trending_matchups_snapshot AS
WITH hourly_votes AS (
    SELECT m.id AS matchup_id, COUNT(*) AS votes
    FROM public.matchup_votes mv
    JOIN public.matchups m ON m.public_id = mv.matchup_public_id
    WHERE mv.created_at >= NOW() - INTERVAL '1 hour'
      AND (mv.user_id IS NULL OR mv.user_id <> m.author_id)
    GROUP BY m.id
),
hourly_likes AS (
    SELECT l.matchup_id, COUNT(*) AS likes
    FROM public.likes l
    JOIN public.matchups m ON m.id = l.matchup_id
    WHERE l.created_at >= NOW() - INTERVAL '1 hour'
      AND l.user_id <> m.author_id
    GROUP BY l.matchup_id
),
hourly_comments AS (
    SELECT c.matchup_id, COUNT(*) AS comments
    FROM public.comments c
    JOIN public.matchups m ON m.id = c.matchup_id
    WHERE c.created_at >= NOW() - INTERVAL '1 hour'
      AND c.user_id <> m.author_id
    GROUP BY c.matchup_id
),
scored AS (
    SELECT
        m.id,
        m.title,
        m.author_id,
        m.bracket_id,
        b.author_id AS bracket_author_id,
        m.round,
        b.current_round,
        COALESCE(hv.votes, 0)::bigint AS votes,
        COALESCE(hl.likes, 0)::bigint AS likes,
        COALESCE(hc.comments, 0)::bigint AS comments,
        (COALESCE(hl.likes, 0) * 3.0
         + COALESCE(hc.comments, 0) * 0.5
         + COALESCE(hv.votes, 0) * 2.0)::float8 AS engagement_score
    FROM public.matchups m
    LEFT JOIN public.brackets   b  ON b.id = m.bracket_id
    LEFT JOIN hourly_votes      hv ON hv.matchup_id = m.id
    LEFT JOIN hourly_likes      hl ON hl.matchup_id = m.id
    LEFT JOIN hourly_comments   hc ON hc.matchup_id = m.id
    WHERE m.status IN ('active', 'published')
)
SELECT
    id,
    title,
    author_id,
    bracket_id,
    bracket_author_id,
    round,
    current_round,
    votes,
    likes,
    comments,
    engagement_score,
    ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM scored
WHERE engagement_score > 0
ORDER BY engagement_score DESC, id ASC
LIMIT 9;

CREATE UNIQUE INDEX idx_trending_matchups_snapshot_id
    ON public.trending_matchups_snapshot (id);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS public.trending_matchups_snapshot;
