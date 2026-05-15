-- +goose Up
-- 029_user_theme_gradient.sql — add a theme_gradient slug to users so
-- viewers can pick a curated gradient for their own profile page.
-- Symmetric with the community theme_gradient column (migration 028):
-- the slug is the wire contract; the raw CSS gradient palette lives
-- in the frontend (frontend/src/utils/communityGradients.js — reused
-- so communities and users always pick from the same set). The
-- backend handler validates against the same allow-list so adding a
-- new gradient is a frontend-palette + server-allowlist tweak, not a
-- migration.
--
-- Default '' keeps existing rows undisturbed — they'll render with
-- the global stardust default until the user picks a theme.

ALTER TABLE public.users
  ADD COLUMN theme_gradient text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.users
  DROP COLUMN IF EXISTS theme_gradient;
