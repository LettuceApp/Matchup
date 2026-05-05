-- +goose Up
-- Email verification (soft-nudge mode).
--
-- `users.email_verified_at` is the source of truth for "is this email
-- confirmed?". NULL on signup; stamped at the moment the user clicks
-- the magic link. Unverified users can still log in + browse — the
-- soft gate in the handlers only blocks 3 specific write paths
-- (CreateMatchup, CreateBracket, CreateComment on other people's
-- content). That keeps App Store review bots happy (they can log in
-- and click around) while frustrating spam networks (they need real
-- inboxes to create content at scale).
--
-- `email_verification_tokens` holds ONE active token per live request.
-- Token value lives in base64url-encoded 32-byte random; at rest we
-- store only SHA-256(token) so a database dump can't be weaponised.
-- The 24 h TTL is baked into `expires_at` rather than a global
-- constant so tests can mint past-dated rows without mocking time.
--
-- ON DELETE CASCADE from users.id means the hard-delete cron
-- auto-cleans. Partial index on WHERE used_at IS NULL keeps the
-- "resend verification" hot-path cheap even after years of
-- accumulated single-use rows.

ALTER TABLE public.users
    ADD COLUMN email_verified_at timestamptz NULL;

CREATE TABLE public.email_verification_tokens (
    id          bigserial   PRIMARY KEY,
    user_id     bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    token_hash  text        NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT NOW(),
    used_at     timestamptz NULL
);
CREATE INDEX idx_email_verification_tokens_user_live
    ON public.email_verification_tokens (user_id)
    WHERE used_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS public.email_verification_tokens;
ALTER TABLE public.users DROP COLUMN IF EXISTS email_verified_at;
