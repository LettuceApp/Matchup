-- +goose Up
-- 028_community_theme_gradient.sql — add a theme_gradient slug to
-- communities so owners can pick a curated gradient from the settings
-- page. The chosen gradient is rendered as the banner background
-- fallback (when no banner image is uploaded) and as the
-- non-image avatar background, so a community has a recognisable
-- look-and-feel even before its owner uploads any artwork.
--
-- Storage: small text slug ('stardust' / 'sunset' / etc.). Empty
-- string means "no theme chosen" and the frontend falls back to the
-- default stardust palette. We deliberately do NOT store the raw
-- CSS gradient — the slug is the contract; the gradient palette
-- lives in the frontend so we can iterate on colors without a
-- migration. A check constraint isn't added for the same reason:
-- adding a new gradient slug shouldn't require a schema change. The
-- handler validates against the allow-list before writing.
--
-- Default '' (empty) means existing rows aren't disturbed — they'll
-- continue to render with the default palette until an owner picks.

ALTER TABLE public.communities
  ADD COLUMN theme_gradient text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.communities
  DROP COLUMN IF EXISTS theme_gradient;
