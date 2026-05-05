-- +goose Up
-- Weekly email digest (Step 7 of the notification roadmap). Default
-- enrollment is ON per product decision — every user gets the Sunday
-- email unless they mute it. The `email_digest_last_sent_at` column is
-- the idempotency guard: the scheduler skips users whose last send was
-- within the last 6 days, so a retried/crashed job won't double-send.
--
-- Opt-out state is stored in the existing `notification_prefs` JSONB
-- (migration 017) as a new key `email_digest`. Backfill every existing
-- row to include the key at default true; new rows created after this
-- migration pick up the updated column default.
--
-- Categories table (kept in code comments, migration 017 for reference):
--   mention | engagement | milestone | prompt | social | email_digest
--
-- We don't model email-digest-specific content in a separate table —
-- the digest is composed fresh per-user at send time from the existing
-- notifications + derived activity helpers.

-- 1. Idempotency timestamp column.
ALTER TABLE public.users
    ADD COLUMN email_digest_last_sent_at timestamptz NULL;

-- 2. Updated default for new rows.
ALTER TABLE public.users
    ALTER COLUMN notification_prefs SET DEFAULT
    '{"mention":true,"engagement":true,"milestone":true,"prompt":true,"social":true,"email_digest":true}'::jsonb;

-- 3. Backfill existing rows — set email_digest=true on everyone. The
--    jsonb_set is idempotent (CREATE_MISSING=true, last arg) so a
--    re-run of this migration is a no-op for already-set rows.
UPDATE public.users
   SET notification_prefs = jsonb_set(
       COALESCE(notification_prefs, '{}'::jsonb),
       '{email_digest}',
       'true'::jsonb,
       true
   )
 WHERE notification_prefs IS NULL
    OR NOT (notification_prefs ? 'email_digest');

-- +goose Down
ALTER TABLE public.users
    ALTER COLUMN notification_prefs SET DEFAULT
    '{"mention":true,"engagement":true,"milestone":true,"prompt":true,"social":true}'::jsonb;

UPDATE public.users
   SET notification_prefs = notification_prefs - 'email_digest';

ALTER TABLE public.users
    DROP COLUMN IF EXISTS email_digest_last_sent_at;
