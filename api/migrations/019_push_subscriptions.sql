-- +goose Up
-- Web Push subscription registry (Step 8 of the notification roadmap).
-- Each browser subscription is one row: users may have several (desktop
-- Chrome + phone Chrome + etc.). The `endpoint` URL is the stable
-- unique identifier provided by the browser's push service.
--
-- Scope — per product decision, pushes fire only for the four priority
-- notification kinds (mentions, milestones, closing-soon, tie-resolution).
-- Opt-in is strictly user-initiated from the settings panel; there is
-- no auto-prompt on sign-in.
--
-- When a push delivery returns 410 Gone (browser unsubscribed or
-- reinstalled), the publisher deletes the row inline. Dead subscriptions
-- don't need a separate cleanup job.

CREATE TABLE public.push_subscriptions (
    id           bigserial    PRIMARY KEY,
    user_id      bigint       NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    endpoint     text         NOT NULL,
    p256dh_key   text         NOT NULL,
    auth_key     text         NOT NULL,
    user_agent   text         NULL,
    created_at   timestamptz  NOT NULL DEFAULT NOW(),
    last_used_at timestamptz  NULL
);

-- Endpoint is globally unique (the browser's push service URL). A user
-- re-subscribing from the same browser updates the existing row rather
-- than creating a duplicate — we ON CONFLICT DO UPDATE in the handler.
CREATE UNIQUE INDEX idx_push_subscriptions_endpoint
    ON public.push_subscriptions (endpoint);

-- Read-path index for the publisher: SELECT all subs for user=$1.
CREATE INDEX idx_push_subscriptions_user
    ON public.push_subscriptions (user_id);

-- +goose Down
DROP TABLE IF EXISTS public.push_subscriptions;
