-- +goose Up
-- Fix: home_new_this_week_snapshot was missing a status filter, allowing
-- draft/completed/archived matchups to appear on the homepage.

DROP MATERIALIZED VIEW IF EXISTS public.home_new_this_week_snapshot;

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
  AND status IN ('active', 'published')
ORDER BY created_at DESC, id DESC
LIMIT 6;

CREATE UNIQUE INDEX idx_home_new_this_week_snapshot_id
    ON public.home_new_this_week_snapshot (id);

-- +goose Down
DROP MATERIALIZED VIEW IF EXISTS public.home_new_this_week_snapshot;

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
