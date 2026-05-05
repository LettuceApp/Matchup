-- +goose Up
-- Refresh tokens — backing store for the access + refresh auth flow.
-- Pre-mobile-apps rewrite: the single-JWT "token is forever" model
-- can't survive phones that cache credentials in Keychain/Keystore
-- for weeks at a time, and has no story for revocation after
-- password change / device loss / session theft.
--
-- Shape notes:
--   token_hash   — sha256(plaintext) hex. Plaintext is 32 random bytes
--                  base64url-encoded; plaintext is never stored.
--   family_id    — every rotation chain shares one UUID. Reusing an
--                  already-consumed token in the family trips the
--                  theft-detection path: every row with that family_id
--                  is revoked in a single UPDATE.
--   previous_id  — self-FK for audit tracing. ON DELETE SET NULL so
--                  the cleanup cron can prune old rows without
--                  breaking the chain for newer ones.
--   used_at      — stamped the moment this token is exchanged for a
--                  new one. A row with used_at=NULL is the active
--                  tip of its family.
--   revoked_at   — stamped by Logout / LogoutAll / theft detection.
--                  Revoked rows stay around for 30 days (see cleanup
--                  cron) so we can answer "why was I logged out?"
--                  support queries.

CREATE TABLE public.refresh_tokens (
    id           bigserial   PRIMARY KEY,
    user_id      bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    token_hash   text        NOT NULL,
    family_id    uuid        NOT NULL,
    previous_id  bigint      NULL REFERENCES public.refresh_tokens(id) ON DELETE SET NULL,
    user_agent   text        NULL,
    created_at   timestamptz NOT NULL DEFAULT NOW(),
    expires_at   timestamptz NOT NULL,
    used_at      timestamptz NULL,
    revoked_at   timestamptz NULL
);

-- Lookup path: find a token by its hash on every Refresh call.
CREATE UNIQUE INDEX idx_refresh_tokens_hash ON public.refresh_tokens (token_hash);

-- "How many live sessions does this user have?" — partial index so we
-- only carry non-revoked, non-used tips in the hot index.
CREATE INDEX idx_refresh_tokens_user_active ON public.refresh_tokens (user_id)
    WHERE revoked_at IS NULL AND used_at IS NULL;

-- Theft detection + audit: scan all rotations sharing a family.
CREATE INDEX idx_refresh_tokens_family ON public.refresh_tokens (family_id);

-- +goose Down
DROP TABLE IF EXISTS public.refresh_tokens;
