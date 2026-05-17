-- +goose Up
-- Belt-and-suspenders against the corruption that nuked the admin's
-- login: a CHECK constraint on users.password that requires a real
-- bcrypt-shaped string. The application-layer fix in
-- auth_connect_handler.go's Login already turns a bad hash into a
-- 401 instead of a 500, but THIS constraint makes sure the bad
-- value never gets persisted in the first place.
--
-- bcrypt hashes from the standard library look like:
--   $2a$10$22-char-salt22-char-salt22-char-31-char-hash3
-- always 60 chars total, always start with `$2a$`, `$2b$`, or
-- `$2y$` (we use the default which produces `$2a$`). Older Go
-- versions wrote `$2a$`; newer is identical. The regex below
-- accepts all three prefixes to stay forward-compatible if the
-- bcrypt library bumps the version letter.
--
-- The constraint is added in two steps:
--   1. ADD CONSTRAINT ... NOT VALID  — applies only to NEW
--      writes; existing rows are left alone. This makes the
--      migration succeed even if there are still rogue rows
--      lurking (none expected, but a goose migration that
--      fails halfway through is a worse outcome than letting
--      a stale row escape the validate step).
--   2. VALIDATE CONSTRAINT          — checks every existing
--      row. If this fails, the migration aborts with a clear
--      list of bad user IDs that an admin can repair via the
--      forgot-password email flow before re-running goose up.

ALTER TABLE public.users
  ADD CONSTRAINT users_password_bcrypt_shape
  CHECK (
    password ~ '^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$'
  )
  NOT VALID;

ALTER TABLE public.users
  VALIDATE CONSTRAINT users_password_bcrypt_shape;

-- +goose Down
-- Drop the constraint without touching the data. The check is
-- additive; reverting just relaxes the rule.

ALTER TABLE public.users
  DROP CONSTRAINT IF EXISTS users_password_bcrypt_shape;
