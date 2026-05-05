-- +goose Up
-- Activity feed v2: per-user notification preferences. Stored as a
-- single JSONB column on users so categories can grow without a
-- migration each time. Five categories cover the 13 current kinds:
--
--   mention     → mention_received
--   engagement  → like_received, comment_received, matchup_vote_received
--   milestone   → milestone_reached
--   prompt      → matchup_closing_soon, tie_needs_resolution
--   social      → new_follower, vote_cast, vote_win, vote_loss,
--                 bracket_progress, matchup_completed, bracket_completed
--
-- Defaults: everything ON. Opt-out rather than opt-in — users with
-- pre-existing accounts see their feed unchanged until they choose
-- to mute a category. The DEFAULT clause also covers the case where
-- a row is inserted via paths that don't set this column.

ALTER TABLE public.users
    ADD COLUMN notification_prefs jsonb NOT NULL DEFAULT
    '{"mention":true,"engagement":true,"milestone":true,"prompt":true,"social":true}'::jsonb;

-- +goose Down
ALTER TABLE public.users DROP COLUMN IF EXISTS notification_prefs;
