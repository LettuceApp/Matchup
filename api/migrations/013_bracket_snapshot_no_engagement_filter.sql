-- +goose Up
-- Fix: popular_brackets_snapshot was filtering out newly-created active
-- brackets with 0 engagement (no likes/votes/comments), so a freshly
-- created bracket never showed up on the homepage. The homepage has only
-- one bracket data source — this snapshot — so if a bracket is missing
-- here, it's invisible site-wide. Remove the engagement_score > 0 filter
-- so all active brackets are eligible; the ROW_NUMBER + LIMIT 12 still
-- caps the list at 12 and still prioritizes popular brackets first.

DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

CREATE MATERIALIZED VIEW public.popular_brackets_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
matchup_scores AS (
    SELECT
        m.bracket_id,
        (m.likes_count * 3.0
         + m.comments_count * 0.5
         + COALESCE(iv.votes, 0) * 2.0)::float8 AS engagement_score
    FROM public.matchups m
    LEFT JOIN item_votes iv ON iv.matchup_id = m.id
    WHERE m.bracket_id IS NOT NULL
      AND m.round IS NOT NULL
),
bracket_round_scores AS (
    SELECT
        b.id,
        b.title,
        b.author_id,
        b.current_round,
        COALESCE(SUM(ms.engagement_score), 0)::float8
            + (b.likes_count * 3.0)::float8 AS engagement_score
    FROM public.brackets b
    LEFT JOIN matchup_scores ms ON ms.bracket_id = b.id
    WHERE b.status = 'active'
    GROUP BY b.id, b.title, b.author_id, b.current_round, b.likes_count
)
SELECT
    id,
    title,
    author_id,
    current_round,
    engagement_score,
    ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM bracket_round_scores
ORDER BY engagement_score DESC, id ASC
LIMIT 12;

CREATE UNIQUE INDEX idx_popular_brackets_snapshot_id
    ON public.popular_brackets_snapshot (id);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

CREATE MATERIALIZED VIEW public.popular_brackets_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
matchup_scores AS (
    SELECT
        m.bracket_id,
        (m.likes_count * 3.0
         + m.comments_count * 0.5
         + COALESCE(iv.votes, 0) * 2.0)::float8 AS engagement_score
    FROM public.matchups m
    LEFT JOIN item_votes iv ON iv.matchup_id = m.id
    WHERE m.bracket_id IS NOT NULL
      AND m.round IS NOT NULL
),
bracket_round_scores AS (
    SELECT
        b.id,
        b.title,
        b.author_id,
        b.current_round,
        COALESCE(SUM(ms.engagement_score), 0)::float8
            + (b.likes_count * 3.0)::float8 AS engagement_score
    FROM public.brackets b
    LEFT JOIN matchup_scores ms ON ms.bracket_id = b.id
    WHERE b.status = 'active'
    GROUP BY b.id, b.title, b.author_id, b.current_round, b.likes_count
)
SELECT
    id,
    title,
    author_id,
    current_round,
    engagement_score,
    ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM bracket_round_scores
WHERE engagement_score > 0
ORDER BY engagement_score DESC, id ASC
LIMIT 12;

CREATE UNIQUE INDEX idx_popular_brackets_snapshot_id
    ON public.popular_brackets_snapshot (id);
