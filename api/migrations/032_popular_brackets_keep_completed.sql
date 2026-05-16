-- +goose Up
-- Keep completed brackets in popular_brackets_snapshot so the
-- homepage shows them alongside active ones.
--
-- Migration 015 (and the lineage before it) filtered the MV down to
-- `WHERE b.status = 'active'`. That cut completed brackets off the
-- home feed the instant they finished — hiding the "receipts" a
-- creator's audience usually wants to revisit (who won, the bracket
-- of legends, the closed tournament). Standalone matchups already
-- got the same fix in matchup_connect_handler.go's ListMatchups;
-- this migration matches the brackets path.
--
-- 'draft' is still excluded — drafts are author-private until
-- explicitly activated. Same posture as the matchups feed.

DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

CREATE MATERIALIZED VIEW public.popular_brackets_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
matchup_net_likes AS (
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
    -- The only line that changes vs migration 015. Everything below
    -- the WHERE is identical so engagement scoring stays consistent.
    WHERE b.status IN ('active', 'completed')
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
-- Restore the active-only filter from migration 015.

DROP MATERIALIZED VIEW IF EXISTS public.popular_brackets_snapshot;

CREATE MATERIALIZED VIEW public.popular_brackets_snapshot AS
WITH item_votes AS (
    SELECT matchup_id, COALESCE(SUM(votes), 0)::bigint AS votes
    FROM public.matchup_items
    GROUP BY matchup_id
),
matchup_net_likes AS (
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
