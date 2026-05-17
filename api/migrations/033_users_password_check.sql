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
-- ADD CONSTRAINT ... NOT VALID:
--   Applies to NEW writes only — existing rows are exempt. This
--   lets the migration succeed even if some legacy row still has
--   a non-bcrypt value sitting in its password column (which prod
--   does: the first deploy of this migration tripped over exactly
--   that case and blocked Render from rolling out the application-
--   layer fix that handles those rows gracefully).
--
-- We deliberately do NOT run `VALIDATE CONSTRAINT` here. The earlier
-- two-step version aborted the whole deploy on a single rogue row,
-- which is the wrong trade — the application-layer guard
-- (auth_connect_handler.go's Login) already turns a corrupted
-- hash into a clean 401 + ops log line, so existing offenders are
-- contained. Going forward, this constraint prevents new
-- corruption from being persisted.
--
-- To find + clean up the existing offender(s) post-deploy:
--   SELECT id, username, length(password)
--     FROM users
--    WHERE password !~ '^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$';
-- Then either reset each via the forgot-password flow or run
-- ALTER TABLE ... VALIDATE CONSTRAINT manually once the table is
-- clean.

ALTER TABLE public.users
  ADD CONSTRAINT users_password_bcrypt_shape
  CHECK (
    password ~ '^\$2[aby]\$[0-9]{2}\$[./A-Za-z0-9]{53}$'
  )
  NOT VALID;

-- +goose Down
-- Drop the constraint without touching the data. The check is
-- additive; reverting just relaxes the rule.

ALTER TABLE public.users
  DROP CONSTRAINT IF EXISTS users_password_bcrypt_shape;
