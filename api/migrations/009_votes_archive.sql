-- +goose Up
-- Create a cold-storage table for votes belonging to long-completed matchups.
--
-- Why archive instead of partition (see migration 008 for the partitioning
-- decision tree): matchup_votes carries unique constraints
--   * UNIQUE (user_id, matchup_public_id)
--   * UNIQUE (anon_id, matchup_public_id)
-- Adding `created_at` to the partition key (which would be required for a
-- RANGE partition by date) would force those constraints to include
-- created_at, which in turn would let the same user vote multiple times for
-- the same matchup in different time windows. That's a correctness break,
-- not just a tradeoff. Archival sidesteps the constraint problem entirely.
--
-- Active matchups still need their vote rows for double-vote prevention and
-- vote-switch tracking. Completed matchups only need their final tally,
-- which is already stored in `matchup_items.votes`. The scheduled Go job
-- `jobArchiveMatchupVotes` in api/scheduler/scheduler.go moves rows for
-- matchups that have been completed for at least 90 days.
--
-- The archive table mirrors the live table's columns one-to-one but
-- intentionally drops the unique constraints — the live table guarantees
-- they were satisfied at archival time, and re-enforcing them on the cold
-- table just adds write overhead with no read benefit.

CREATE TABLE public.matchup_votes_archive (
    id                     bigint                   NOT NULL,
    public_id              uuid                     NOT NULL,
    user_id                bigint,
    anon_id                character varying(36),
    matchup_public_id      uuid                     NOT NULL,
    matchup_item_public_id uuid                     NOT NULL,
    created_at             timestamp with time zone,
    updated_at             timestamp with time zone,
    archived_at            timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id)
);

-- Index by matchup so historical lookups (e.g. an admin or a moderation
-- audit on a long-finished matchup) don't need a sequential scan.
CREATE INDEX idx_matchup_votes_archive_matchup_public_id
    ON public.matchup_votes_archive (matchup_public_id);

-- Index by user for "show me everything I ever voted on" historical reads.
CREATE INDEX idx_matchup_votes_archive_user_id
    ON public.matchup_votes_archive (user_id)
    WHERE user_id IS NOT NULL;

-- Index by archived_at so the cleanup cron in Step 29 can drop very old
-- archived rows efficiently.
CREATE INDEX idx_matchup_votes_archive_archived_at
    ON public.matchup_votes_archive (archived_at);

-- +goose Down
DROP TABLE IF EXISTS public.matchup_votes_archive;
