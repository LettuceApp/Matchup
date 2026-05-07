-- +goose Up
-- Item thumbnails.
--
-- Pairwise-comparison research (the same set that drove the side-by-side
-- layout + Skip button) is consistent that visual richness on contender
-- cards measurably improves recognizability — a Blade Runner card with
-- the actual Blade Runner poster reads instantly, whereas a card with
-- just the words "Blade Runner 2049" forces a moment of cognitive
-- translation. Adding image_path here gives the frontend a thumbnail
-- to render alongside the existing text label.
--
-- Mirrors `matchups.image_path` on purpose: same column type, same
-- empty-string default, same upload pipeline (presigned PUT to S3,
-- resized variants on commit). The proto layer lifts the relative path
-- to a full URL via ProcessMatchupItemImagePath in db.go.
--
-- Existing items backfill to '' (empty) via the column default —
-- non-breaking. Frontend treats empty as "no thumbnail" and falls back
-- to the existing text-only card. So this migration plus a redeploy
-- shows no UI change until users start picking images on creation.

ALTER TABLE public.matchup_items
    ADD COLUMN image_path text NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE public.matchup_items
    DROP COLUMN IF EXISTS image_path;
