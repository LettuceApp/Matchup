-- +goose Up
-- Account deletion + ban lifecycle. `deleted_at` is the single flag
-- that hides a user everywhere (profile, comments, activity, feeds);
-- `banned_at` distinguishes an admin ban from a self-delete for
-- moderation audit purposes but is NOT surfaced to other users.
--
-- Both `deleted_at` paths are soft — a daily cron (registered in
-- the scheduler) does the hard delete 30 days later via the shared
-- `hardDeleteUser` cascade so self-delete and admin-delete converge
-- on the same cleanup code.
--
-- Partial index on deleted_at: cron scans only the rows that need
-- scanning. 99%+ of the users table stays out of the index.

ALTER TABLE public.users
    ADD COLUMN deleted_at      timestamptz NULL,
    ADD COLUMN deletion_reason text         NULL,
    ADD COLUMN banned_at       timestamptz NULL,
    ADD COLUMN ban_reason      text         NULL;

CREATE INDEX idx_users_deleted_at ON public.users (deleted_at)
    WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_users_deleted_at;
ALTER TABLE public.users
    DROP COLUMN IF EXISTS ban_reason,
    DROP COLUMN IF EXISTS banned_at,
    DROP COLUMN IF EXISTS deletion_reason,
    DROP COLUMN IF EXISTS deleted_at;
