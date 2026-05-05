-- +goose Up
-- Moderation primitives: user-to-user blocks + mutes, user-submitted
-- content reports, and a moderator-action audit log.
--
-- Schema shape notes:
-- * `user_blocks` / `user_mutes` mirror `follows` — a two-column
--   directed-edge table with a unique index on the pair and a
--   self-edge CHECK. `follows` doesn't CASCADE on user delete, but
--   these do (a deleted user's block/mute rows are semantically
--   meaningless once the row is gone).
-- * `reports` uses CHECK constraints on subject_type / reason /
--   status / resolution so the handler + the DB agree on the closed
--   vocabulary. If we extend either set, the migration + the handler
--   change together.
-- * `admin_actions` is append-only — no updates, no deletes. Any
--   reviewer "undo" is a new row with action='unban' / etc.

-- ---------------------------------------------------------------------
-- user_blocks: bidirectional hide. Block filter returns union of
-- rows where viewer is blocker OR viewer is blocked.
-- ---------------------------------------------------------------------
CREATE TABLE public.user_blocks (
    id          bigserial   PRIMARY KEY,
    blocker_id  bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    blocked_id  bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT  user_blocks_no_self CHECK (blocker_id <> blocked_id)
);
CREATE UNIQUE INDEX idx_user_blocks_unique  ON public.user_blocks (blocker_id, blocked_id);
CREATE INDEX        idx_user_blocks_blocker ON public.user_blocks (blocker_id);
CREATE INDEX        idx_user_blocks_blocked ON public.user_blocks (blocked_id);

-- ---------------------------------------------------------------------
-- user_mutes: one-way hide in muter's activity feed + push delivery.
-- ---------------------------------------------------------------------
CREATE TABLE public.user_mutes (
    id          bigserial   PRIMARY KEY,
    muter_id    bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    muted_id    bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT NOW(),
    CONSTRAINT  user_mutes_no_self CHECK (muter_id <> muted_id)
);
CREATE UNIQUE INDEX idx_user_mutes_unique ON public.user_mutes (muter_id, muted_id);
CREATE INDEX        idx_user_mutes_muter  ON public.user_mutes (muter_id);

-- ---------------------------------------------------------------------
-- reports: user-submitted content reports awaiting admin review.
-- reporter_id is SET NULL on user delete so the report doesn't
-- vanish along with the reporter (useful for patterns across
-- multiple deleted accounts).
-- ---------------------------------------------------------------------
CREATE TABLE public.reports (
    id              bigserial   PRIMARY KEY,
    reporter_id     bigint      NULL REFERENCES public.users(id) ON DELETE SET NULL,
    subject_type    text        NOT NULL CHECK (subject_type IN (
                        'matchup','bracket','comment','bracket_comment','user'
                    )),
    subject_id      bigint      NOT NULL,
    reason          text        NOT NULL CHECK (reason IN (
                        'harassment','spam','violence','nudity',
                        'misinformation','self_harm','copyright','other'
                    )),
    reason_detail   text        NULL,
    status          text        NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open','resolved')),
    resolution      text        NULL CHECK (resolution IS NULL OR resolution IN (
                        'dismiss','remove_content','warn_user','ban_user'
                    )),
    created_at      timestamptz NOT NULL DEFAULT NOW(),
    reviewed_at     timestamptz NULL,
    reviewed_by     bigint      NULL REFERENCES public.users(id) ON DELETE SET NULL
);

-- Partial index: the Reports tab in AdminDashboard scans open rows
-- constantly; resolved rows are rare queries (audit lookups). Keeping
-- the hot index narrow keeps writes cheap.
CREATE INDEX idx_reports_open     ON public.reports (created_at DESC) WHERE status = 'open';
CREATE INDEX idx_reports_subject  ON public.reports (subject_type, subject_id);
CREATE INDEX idx_reports_reporter ON public.reports (reporter_id);

-- ---------------------------------------------------------------------
-- admin_actions: append-only moderation audit log. Populated by every
-- reviewer action (ResolveReport, BanUser, UnbanUser, RemoveContent).
-- Also a compliance surface — "show me every action this moderator
-- ever took" is a cheap (actor_id, created_at) scan.
-- ---------------------------------------------------------------------
CREATE TABLE public.admin_actions (
    id           bigserial   PRIMARY KEY,
    actor_id     bigint      NULL REFERENCES public.users(id) ON DELETE SET NULL,
    action       text        NOT NULL,
    subject_type text        NOT NULL,
    subject_id   bigint      NOT NULL,
    notes        text        NULL,
    created_at   timestamptz NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_admin_actions_subject ON public.admin_actions (subject_type, subject_id);
CREATE INDEX idx_admin_actions_actor   ON public.admin_actions (actor_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS public.admin_actions;
DROP TABLE IF EXISTS public.reports;
DROP TABLE IF EXISTS public.user_mutes;
DROP TABLE IF EXISTS public.user_blocks;
