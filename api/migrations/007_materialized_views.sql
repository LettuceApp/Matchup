-- +goose Up
-- Replace the snapshot tables previously written by the Pandas pipeline with
-- true PostgreSQL MATERIALIZED VIEWs. The views read from the denormalized
-- counter columns added in migration 006, so they avoid the per-row COUNT(*)
-- traffic the Pandas job had to do. Each view has a UNIQUE INDEX so it can be
-- refreshed via REFRESH MATERIALIZED VIEW CONCURRENTLY without blocking reads.

DROP TABLE IF EXISTS public.popular_matchups_snapshot CASCADE;
DROP TABLE IF EXISTS public.popular_brackets_snapshot CASCADE;
DROP TABLE IF EXISTS public.home_summary_snapshot CASCADE;
DROP TABLE IF EXISTS public.home_new_this_week_snapshot CASCADE;
DROP TABLE IF EXISTS public.home_creators_snapshot CASCADE;

-- ---------------------------------------------------------------------------
-- popular_matchups_snapshot
-- ---------------------------------------------------------------------------
-- Top 5 matchups by engagement_score = likes*3 + comments*0.5 + votes*2.
-- The Go scheduler (api/scheduler/scheduler.go::jobRefreshSnapshots) runs
-- `REFRESH MATERIALIZED VIEW CONCURRENTLY` on this view every minute.

CREATE MATERIALIZED VIEW public.popular_matchups_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
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
        COALESCE(iv.votes, 0)::bigint AS votes,
        m.likes_count::bigint        AS likes,
        m.comments_count::bigint     AS comments,
        (m.likes_count * 3.0
         + m.comments_count * 0.5
         + COALESCE(iv.votes, 0) * 2.0)::float8 AS engagement_score
    FROM public.matchups m
    LEFT JOIN public.brackets   b  ON b.id  = m.bracket_id
    LEFT JOIN item_votes        iv ON iv.matchup_id = m.id
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
LIMIT 5;

CREATE UNIQUE INDEX idx_popular_matchups_snapshot_id
    ON public.popular_matchups_snapshot (id);

-- ---------------------------------------------------------------------------
-- popular_brackets_snapshot
-- ---------------------------------------------------------------------------
-- Top 12 active brackets by aggregated current-round matchup engagement plus
-- bracket likes*3. Sized at 12 because home_connect_handler.go::GetHomeSummary
-- queries `LIMIT 12` from this view.

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

-- ---------------------------------------------------------------------------
-- home_summary_snapshot
-- ---------------------------------------------------------------------------
-- Single-row materialized view with the global counters used on the home
-- page. votes_today is intentionally a 24h rolling window to match the
-- previous Pandas implementation.

CREATE MATERIALIZED VIEW public.home_summary_snapshot AS
SELECT
    (SELECT COUNT(*) FROM public.matchup_votes
        WHERE created_at >= NOW() - INTERVAL '1 day')::bigint    AS votes_today,
    (SELECT COUNT(*) FROM public.matchups
        WHERE status IN ('active', 'published'))::bigint         AS active_matchups,
    (SELECT COUNT(*) FROM public.brackets
        WHERE status = 'active')::bigint                         AS active_brackets,
    1::int AS singleton;

CREATE UNIQUE INDEX idx_home_summary_snapshot_singleton
    ON public.home_summary_snapshot (singleton);

-- ---------------------------------------------------------------------------
-- home_new_this_week_snapshot
-- ---------------------------------------------------------------------------
-- Up to 6 standalone matchups (bracket_id IS NULL) created in the past 7
-- days, ordered most recent first.

CREATE MATERIALIZED VIEW public.home_new_this_week_snapshot AS
SELECT
    id,
    title,
    author_id,
    bracket_id,
    visibility,
    created_at,
    ROW_NUMBER() OVER (ORDER BY created_at DESC, id DESC)::int AS rank
FROM public.matchups
WHERE created_at >= NOW() - INTERVAL '7 days'
  AND bracket_id IS NULL
ORDER BY created_at DESC, id DESC
LIMIT 6;

CREATE UNIQUE INDEX idx_home_new_this_week_snapshot_id
    ON public.home_new_this_week_snapshot (id);

-- ---------------------------------------------------------------------------
-- home_creators_snapshot
-- ---------------------------------------------------------------------------
-- Top 10 non-admin creators by followers_count. Used for the "creators to
-- follow" home section.

CREATE MATERIALIZED VIEW public.home_creators_snapshot AS
SELECT
    id,
    username,
    avatar_path,
    followers_count,
    following_count,
    is_private,
    ROW_NUMBER() OVER (ORDER BY followers_count DESC, id ASC)::int AS rank
FROM public.users
WHERE is_admin = false
ORDER BY followers_count DESC, id ASC
LIMIT 10;

CREATE UNIQUE INDEX idx_home_creators_snapshot_id
    ON public.home_creators_snapshot (id);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS public.home_creators_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.home_new_this_week_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.home_summary_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.popular_matchups_snapshot;
