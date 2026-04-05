-- +goose Up
ALTER TABLE public.users ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.users DROP COLUMN IF EXISTS bio;
