-- +goose Up
-- Activity feed v2: the `notifications` table is the home for persisted,
-- one-shot events that don't have a natural source row in an existing
-- table. The derived-feed helpers in activity_connect_handler.go cover
-- "someone did X to your content" just fine by querying likes /
-- comments / follows / matchup_votes directly. But milestones ("your
-- matchup hit 100 votes"), closing-soon prompts, and tie-resolution
-- prompts have no write event that corresponds 1:1 — we have to
-- materialise them as a scheduler decides to fire once per crossing.
--
-- The unique index on (user_id, kind, subject_type, subject_id,
-- COALESCE(threshold_int, -1)) enforces fire-once semantics: re-running
-- the milestone scanner every 15 minutes is a no-op after the first
-- crossing, and closing-soon scanners can re-run without stacking
-- duplicate rows. The COALESCE trick lets non-milestone kinds (which
-- leave threshold_int NULL) still use the unique predicate without
-- needing a separate index.
--
-- subject_id is `bigint` referring to the INTERNAL id of the subject
-- (matchup / bracket / user id, NOT public_id). The handler read path
-- resolves it to a public UUID via the existing loadMatchupPublicIDMap /
-- loadBracketPublicIDMap / loadUserPublicIDMap helpers so the frontend
-- still sees UUIDs end-to-end.
--
-- payload is jsonb for forward-compat with future kinds that need extra
-- fields (threshold metric name, winner item label, etc.) without a
-- migration per kind.

CREATE TABLE public.notifications (
    id            bigserial PRIMARY KEY,
    user_id       bigint       NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    kind          text         NOT NULL,
    subject_type  text         NOT NULL,
    subject_id    bigint       NOT NULL,
    threshold_int integer      NULL,
    payload       jsonb        NOT NULL DEFAULT '{}'::jsonb,
    occurred_at   timestamptz  NOT NULL DEFAULT NOW(),
    read_at       timestamptz  NULL
);

-- Dedupe index — the scheduler inserts with ON CONFLICT DO NOTHING and
-- this index is the predicate. COALESCE lets us share one unique index
-- across milestone and non-milestone kinds.
CREATE UNIQUE INDEX idx_notifications_dedupe
    ON public.notifications (user_id, kind, subject_type, subject_id, COALESCE(threshold_int, -1));

-- Read-path index — the handler's loadNotificationsActivity does a
-- per-user SELECT ordered by occurred_at DESC.
CREATE INDEX idx_notifications_user_occurred
    ON public.notifications (user_id, occurred_at DESC);

-- +goose Down
DROP TABLE IF EXISTS public.notifications;
