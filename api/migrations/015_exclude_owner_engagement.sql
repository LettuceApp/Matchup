-- +goose Up
-- An owner's own vote / like / comment should still count toward the
-- visible tally on the matchup (so contender bars and "N likes"
-- display accurately) but MUST NOT contribute to the engagement_score
-- that drives trending / popular / most-played ranking. Otherwise an
-- owner can artificially inflate their own content into the feed.
--
-- Migrations 010 (trending) and 011 (most_played) already filter owner
-- interactions out of their engagement calcs by joining the source
-- tables (likes / comments / matchup_votes) with a
-- `user_id <> author_id` predicate. The popular_matchups_snapshot and
-- popular_brackets_snapshot from migration 007 did not — they used
-- denormalized counter columns (`likes_count`, `comments_count`,
-- `matchup_items.votes`) which include every interaction regardless of
-- author. This migration recreates both views with the same
-- owner-exclusion pattern the hourly MVs use.
--
-- Display columns (`votes`, `likes`, `comments`) still reflect the raw
-- totals, so cards and engagement strips keep showing "5 likes" even
-- when one of them is the author's own like. Only the internal
-- `engagement_score` changes.

DROP MATERIALIZED VIEW IF EXISTS public.popular_matchups_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

-- ---------------------------------------------------------------------------
-- popular_matchups_snapshot (replaces migration 007's version)
-- ---------------------------------------------------------------------------

CREATE MATERIALIZED VIEW public.popular_matchups_snapshot AS
WITH item_votes AS (
    -- Display vote total per matchup. Keeps owner vote included so the
    -- contender bar math on the card remains accurate.
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
net_likes AS (
    -- Owner-excluded like count, used ONLY for engagement_score.
    SELECT l.matchup_id, COUNT(*)::bigint AS n
    FROM public.likes l
    JOIN public.matchups m ON m.id = l.matchup_id
    WHERE l.user_id <> m.author_id
    GROUP BY l.matchup_id
),
net_comments AS (
    SELECT c.matchup_id, COUNT(*)::bigint AS n
    FROM public.comments c
    JOIN public.matchups m ON m.id = c.matchup_id
    WHERE c.user_id <> m.author_id
    GROUP BY c.matchup_id
),
net_votes AS (
    -- matchup_votes carries user_id nullable (anon votes). Both
    -- anonymous and non-owner authenticated votes should count toward
    -- engagement; only votes from the matchup's author are excluded.
    SELECT m.id AS matchup_id, COUNT(*)::bigint AS n
    FROM public.matchup_votes mv
    JOIN public.matchups m ON m.public_id = mv.matchup_public_id
    WHERE mv.user_id IS NULL OR mv.user_id <> m.author_id
    GROUP BY m.id
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
        COALESCE(iv.votes, 0)::bigint        AS votes,      -- display
        m.likes_count::bigint                AS likes,      -- display
        m.comments_count::bigint             AS comments,   -- display
        (COALESCE(nl.n, 0) * 3.0
         + COALESCE(nc.n, 0) * 0.5
         + COALESCE(nv.n, 0) * 2.0)::float8  AS engagement_score  -- owner-excluded
    FROM public.matchups m
    LEFT JOIN public.brackets   b  ON b.id  = m.bracket_id
    LEFT JOIN item_votes        iv ON iv.matchup_id = m.id
    LEFT JOIN net_likes         nl ON nl.matchup_id = m.id
    LEFT JOIN net_comments      nc ON nc.matchup_id = m.id
    LEFT JOIN net_votes         nv ON nv.matchup_id = m.id
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
-- popular_brackets_snapshot (replaces migration 013's version)
-- ---------------------------------------------------------------------------
-- Preserves migration 013's change — no `engagement_score > 0` filter,
-- so newly-created active brackets surface before they accrue third-
-- party engagement. Only the engagement-score calc changes here.

CREATE MATERIALIZED VIEW public.popular_brackets_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
matchup_net_likes AS (
    -- Likes on a bracket's child matchups minus any from the bracket's
    -- own author (since auto-generated bracket matchups inherit the
    -- bracket author's identity). Keyed by bracket_id so the outer
    -- sum groups by bracket cleanly.
    SELECT m.bracket_id, COUNT(*)::bigint AS n
    FROM public.likes l
    JOIN public.matchups m ON m.id = l.matchup_id
    JOIN public.brackets b ON b.id = m.bracket_id
    WHERE m.bracket_id IS NOT NULL
      AND l.user_id <> b.author_id
    GROUP BY m.bracket_id
),
matchup_net_comments AS (
    SELECT m.bracket_id, COUNT(*)::bigint AS n
    FROM public.comments c
    JOIN public.matchups m ON m.id = c.matchup_id
    JOIN public.brackets b ON b.id = m.bracket_id
    WHERE m.bracket_id IS NOT NULL
      AND c.user_id <> b.author_id
    GROUP BY m.bracket_id
),
matchup_net_votes AS (
    SELECT m.bracket_id, COUNT(*)::bigint AS n
    FROM public.matchup_votes mv
    JOIN public.matchups m ON m.public_id = mv.matchup_public_id
    JOIN public.brackets b ON b.id = m.bracket_id
    WHERE m.bracket_id IS NOT NULL
      AND (mv.user_id IS NULL OR mv.user_id <> b.author_id)
    GROUP BY m.bracket_id
),
bracket_net_likes AS (
    -- Likes on the bracket itself, minus any self-like from the
    -- bracket author.
    SELECT bl.bracket_id, COUNT(*)::bigint AS n
    FROM public.bracket_likes bl
    JOIN public.brackets b ON b.id = bl.bracket_id
    WHERE bl.user_id <> b.author_id
    GROUP BY bl.bracket_id
),
scored AS (
    SELECT
        b.id,
        b.title,
        b.author_id,
        b.current_round,
        (COALESCE(mnl.n, 0) * 3.0
         + COALESCE(mnc.n, 0) * 0.5
         + COALESCE(mnv.n, 0) * 2.0
         + COALESCE(bnl.n, 0) * 3.0)::float8 AS engagement_score
    FROM public.brackets b
    LEFT JOIN matchup_net_likes    mnl ON mnl.bracket_id = b.id
    LEFT JOIN matchup_net_comments mnc ON mnc.bracket_id = b.id
    LEFT JOIN matchup_net_votes    mnv ON mnv.bracket_id = b.id
    LEFT JOIN bracket_net_likes    bnl ON bnl.bracket_id = b.id
    WHERE b.status = 'active'
)
SELECT
    id,
    title,
    author_id,
    current_round,
    engagement_score,
    ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM scored
ORDER BY engagement_score DESC, id ASC
LIMIT 12;

CREATE UNIQUE INDEX idx_popular_brackets_snapshot_id
    ON public.popular_brackets_snapshot (id);

-- +goose Down
-- Restore migration 013 + migration 007 popular MV definitions.

DROP MATERIALIZED VIEW IF EXISTS public.popular_matchups_snapshot;
DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

CREATE MATERIALIZED VIEW public.popular_matchups_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
scored AS (
    SELECT
        m.id, m.title, m.author_id, m.bracket_id,
        b.author_id AS bracket_author_id,
        m.round, b.current_round,
        COALESCE(iv.votes, 0)::bigint AS votes,
        m.likes_count::bigint         AS likes,
        m.comments_count::bigint      AS comments,
        (m.likes_count * 3.0
         + m.comments_count * 0.5
         + COALESCE(iv.votes, 0) * 2.0)::float8 AS engagement_score
    FROM public.matchups m
    LEFT JOIN public.brackets b ON b.id = m.bracket_id
    LEFT JOIN item_votes iv ON iv.matchup_id = m.id
    WHERE m.status IN ('active', 'published')
)
SELECT id, title, author_id, bracket_id, bracket_author_id,
       round, current_round, votes, likes, comments, engagement_score,
       ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM scored
WHERE engagement_score > 0
ORDER BY engagement_score DESC, id ASC
LIMIT 5;

CREATE UNIQUE INDEX idx_popular_matchups_snapshot_id
    ON public.popular_matchups_snapshot (id);

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
        b.id, b.title, b.author_id, b.current_round,
        COALESCE(SUM(ms.engagement_score), 0)::float8
            + (b.likes_count * 3.0)::float8 AS engagement_score
    FROM public.brackets b
    LEFT JOIN matchup_scores ms ON ms.bracket_id = b.id
    WHERE b.status = 'active'
    GROUP BY b.id, b.title, b.author_id, b.current_round, b.likes_count
)
SELECT id, title, author_id, current_round, engagement_score,
       ROW_NUMBER() OVER (ORDER BY engagement_score DESC, id ASC)::bigint AS rank
FROM bracket_round_scores
ORDER BY engagement_score DESC, id ASC
LIMIT 12;

CREATE UNIQUE INDEX idx_popular_brackets_snapshot_id
    ON public.popular_brackets_snapshot (id);
