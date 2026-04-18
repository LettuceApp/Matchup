-- +goose Up
CREATE TABLE IF NOT EXISTS public.matchup_details (
    matchup_id bigint PRIMARY KEY REFERENCES public.matchups(id) ON DELETE CASCADE,
    content text NOT NULL DEFAULT ''
);

INSERT INTO public.matchup_details (matchup_id, content)
SELECT id, content FROM public.matchups
ON CONFLICT (matchup_id) DO NOTHING;

ALTER TABLE public.matchups DROP COLUMN IF EXISTS content;

-- +goose Down
ALTER TABLE public.matchups ADD COLUMN IF NOT EXISTS content text NOT NULL DEFAULT '';
UPDATE public.matchups m SET content = d.content FROM public.matchup_details d WHERE m.id = d.matchup_id;
DROP TABLE IF EXISTS public.matchup_details;
