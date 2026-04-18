-- +goose Up
-- Convert public.comments into a RANGE-partitioned table on `created_at`.
--
-- Why partition `comments` (and not `bracket_comments` or `matchup_votes`):
--   * Comments grow unboundedly with engagement and have no global UNIQUE
--     constraint that would force the partition key into the constraint.
--   * Old partitions can be dropped instantly via DETACH+DROP, no VACUUM.
--   * Partition pruning helps the planner for time-bounded analytics queries.
--   * matchup_votes carries `UNIQUE (user_id, matchup_public_id)` and
--     `UNIQUE (anon_id, matchup_public_id)` — adding `created_at` to the
--     partition key would let users vote multiple times. Votes are handled by
--     archival in migration 009 instead.
--
-- This migration briefly takes ACCESS EXCLUSIVE on `comments` while it
-- swaps. Run during low traffic. Existing rows are copied (no data loss);
-- the trigger from migration 006 is dropped and re-attached so the
-- denormalized `matchups.comments_count` keeps working without drift.

DROP TRIGGER IF EXISTS trg_bump_matchup_comments_count ON public.comments;

-- Detach the bigserial sequence from the soon-to-be-dropped column so it
-- survives the table drop and we can re-attach it to the new column below.
ALTER SEQUENCE IF EXISTS public.comments_id_seq OWNED BY NONE;

-- Capture the current max id and 7-day cutoff so the sequence + partitions
-- align with existing data. Stored in a temp table because PL/pgSQL local
-- variables don't survive across statements.
CREATE TEMP TABLE _comments_migration_state AS
SELECT COALESCE(MAX(id), 0) + 1 AS next_id FROM public.comments;

CREATE TABLE public.comments_new (
    id          bigint                   NOT NULL,
    public_id   uuid                     DEFAULT gen_random_uuid() NOT NULL,
    user_id     bigint                   NOT NULL,
    matchup_id  bigint                   NOT NULL,
    body        text                     NOT NULL,
    created_at  timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- ---- partitions -------------------------------------------------------------
-- One historical partition for everything before the rollout date, then
-- monthly partitions for the next year. The Go scheduler in
-- api/scheduler/scheduler.go (jobManageCommentPartitions) is responsible
-- for adding new monthly partitions before the window runs out, and for
-- dropping partitions older than 24 months. The DEFAULT partition is a
-- safety net so inserts never fail; in normal operation it should stay empty.

CREATE TABLE public.comments_historical PARTITION OF public.comments_new
    FOR VALUES FROM ('1970-01-01') TO ('2026-04-01');

CREATE TABLE public.comments_2026_04 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
CREATE TABLE public.comments_2026_05 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');
CREATE TABLE public.comments_2026_06 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-06-01') TO ('2026-07-01');
CREATE TABLE public.comments_2026_07 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-07-01') TO ('2026-08-01');
CREATE TABLE public.comments_2026_08 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-08-01') TO ('2026-09-01');
CREATE TABLE public.comments_2026_09 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-09-01') TO ('2026-10-01');
CREATE TABLE public.comments_2026_10 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-10-01') TO ('2026-11-01');
CREATE TABLE public.comments_2026_11 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-11-01') TO ('2026-12-01');
CREATE TABLE public.comments_2026_12 PARTITION OF public.comments_new
    FOR VALUES FROM ('2026-12-01') TO ('2027-01-01');
CREATE TABLE public.comments_2027_01 PARTITION OF public.comments_new
    FOR VALUES FROM ('2027-01-01') TO ('2027-02-01');
CREATE TABLE public.comments_2027_02 PARTITION OF public.comments_new
    FOR VALUES FROM ('2027-02-01') TO ('2027-03-01');
CREATE TABLE public.comments_2027_03 PARTITION OF public.comments_new
    FOR VALUES FROM ('2027-03-01') TO ('2027-04-01');

CREATE TABLE public.comments_default PARTITION OF public.comments_new DEFAULT;

-- Copy existing rows. created_at must be non-null because it's part of the
-- PK; existing rows already have it set (DEFAULT CURRENT_TIMESTAMP) but
-- COALESCE handles any historical NULL just in case.
INSERT INTO public.comments_new (id, public_id, user_id, matchup_id, body, created_at, updated_at)
SELECT id, public_id, user_id, matchup_id, body, COALESCE(created_at, CURRENT_TIMESTAMP), updated_at
FROM public.comments;

-- Replace the source table with the partitioned one.
DROP TABLE public.comments;
ALTER TABLE public.comments_new RENAME TO comments;

-- Reattach the sequence and reset it past the highest copied id.
ALTER SEQUENCE public.comments_id_seq OWNED BY public.comments.id;
ALTER TABLE public.comments ALTER COLUMN id SET DEFAULT nextval('public.comments_id_seq'::regclass);
SELECT setval(
    'public.comments_id_seq',
    (SELECT next_id FROM _comments_migration_state),
    false
);
DROP TABLE _comments_migration_state;

-- ---- indexes ---------------------------------------------------------------
-- A unique constraint on a partitioned table must include the partition key.
-- The pre-partition idx_comments_public_id was UNIQUE(public_id); we replace
-- it with UNIQUE(public_id, created_at) (UUIDs are collision-resistant so the
-- effective uniqueness is unchanged) plus a non-unique INDEX on public_id
-- alone for lookups by public_id without a created_at filter
-- (resolveCommentByIdentifier in resource_lookup.go).

CREATE UNIQUE INDEX idx_comments_public_id_created_at
    ON public.comments (public_id, created_at);

CREATE INDEX idx_comments_public_id
    ON public.comments (public_id);

CREATE INDEX idx_comments_id
    ON public.comments (id);

CREATE INDEX idx_comments_matchup_id_created_at
    ON public.comments (matchup_id, created_at DESC);

CREATE INDEX idx_comments_user_id
    ON public.comments (user_id);

-- ---- foreign keys ----------------------------------------------------------
ALTER TABLE public.comments
    ADD CONSTRAINT fk_comments_author
        FOREIGN KEY (user_id) REFERENCES public.users(id);
ALTER TABLE public.comments
    ADD CONSTRAINT fk_matchups_comments
        FOREIGN KEY (matchup_id) REFERENCES public.matchups(id);

-- ---- trigger (from migration 006) ------------------------------------------
-- The function still exists; we just need to re-attach the trigger to the
-- new partitioned parent. PostgreSQL 13+ propagates row-level triggers from
-- the partitioned table to all child partitions automatically.
CREATE TRIGGER trg_bump_matchup_comments_count
AFTER INSERT OR DELETE ON public.comments
FOR EACH ROW EXECUTE FUNCTION public.bump_matchup_comments_count();

-- +goose Down
-- Reverse the partitioning by collapsing every partition back into a plain
-- table. The function from migration 006 is preserved.

DROP TRIGGER IF EXISTS trg_bump_matchup_comments_count ON public.comments;

CREATE TABLE public.comments_unpartitioned (
    id          bigserial PRIMARY KEY,
    public_id   uuid                     DEFAULT gen_random_uuid() NOT NULL,
    user_id     bigint                   NOT NULL,
    matchup_id  bigint                   NOT NULL,
    body        text                     NOT NULL,
    created_at  timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at  timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO public.comments_unpartitioned (id, public_id, user_id, matchup_id, body, created_at, updated_at)
SELECT id, public_id, user_id, matchup_id, body, created_at, updated_at FROM public.comments;

SELECT setval(
    'public.comments_unpartitioned_id_seq',
    COALESCE((SELECT MAX(id) FROM public.comments_unpartitioned), 1),
    true
);

DROP TABLE public.comments;
ALTER TABLE public.comments_unpartitioned RENAME TO comments;
ALTER SEQUENCE public.comments_unpartitioned_id_seq RENAME TO comments_id_seq;

CREATE UNIQUE INDEX idx_comments_public_id ON public.comments USING btree (public_id);
CREATE INDEX idx_comments_matchup_id_created_at ON public.comments (matchup_id, created_at DESC);

ALTER TABLE public.comments
    ADD CONSTRAINT fk_comments_author
        FOREIGN KEY (user_id) REFERENCES public.users(id);
ALTER TABLE public.comments
    ADD CONSTRAINT fk_matchups_comments
        FOREIGN KEY (matchup_id) REFERENCES public.matchups(id);

CREATE TRIGGER trg_bump_matchup_comments_count
AFTER INSERT OR DELETE ON public.comments
FOR EACH ROW EXECUTE FUNCTION public.bump_matchup_comments_count();
