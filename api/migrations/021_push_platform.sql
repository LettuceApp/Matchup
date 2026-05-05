-- +goose Up
-- Multi-platform push: add a `platform` discriminator so one
-- push_subscriptions row can represent a web browser, an iOS device
-- (APNS), or an Android device (FCM). Each platform stores its device
-- token in `endpoint`; the p256dh/auth columns are only meaningful
-- for Web Push, so native rows leave them NULL.
--
-- Backfill strategy: every existing row was necessarily a Web Push
-- subscription (the previous schema enforced that), so the DEFAULT
-- 'web' on the new column does the right thing without an explicit
-- UPDATE pass.
--
-- The CHECK constraint keeps the accepted values honest at the DB
-- layer — a handler bug that tried to store 'ipad' or 'macos' would
-- fail loudly rather than silently creating dispatch-side
-- dead-letter rows.

ALTER TABLE public.push_subscriptions
    ADD COLUMN platform text NOT NULL DEFAULT 'web'
    CHECK (platform IN ('web', 'ios', 'android'));

ALTER TABLE public.push_subscriptions ALTER COLUMN p256dh_key DROP NOT NULL;
ALTER TABLE public.push_subscriptions ALTER COLUMN auth_key   DROP NOT NULL;

-- +goose Down
-- NOT reversible for native rows: if any iOS/Android subs landed
-- while this migration was applied, the p256dh/auth SET NOT NULL
-- will fail. That's intentional — rolling back after native
-- subscriptions exist would silently lose them. Drop the column
-- first; operators who've shipped native need to reconcile by hand.
ALTER TABLE public.push_subscriptions DROP COLUMN IF EXISTS platform;
ALTER TABLE public.push_subscriptions ALTER COLUMN p256dh_key SET NOT NULL;
ALTER TABLE public.push_subscriptions ALTER COLUMN auth_key   SET NOT NULL;
