-- +goose Up
ALTER TABLE public.matchups ADD COLUMN IF NOT EXISTS image_path VARCHAR(500) NOT NULL DEFAULT '';
ALTER TABLE public.matchups ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE public.brackets ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE public.matchups DROP COLUMN IF EXISTS image_path;
ALTER TABLE public.matchups DROP COLUMN IF EXISTS tags;
ALTER TABLE public.brackets DROP COLUMN IF EXISTS tags;
