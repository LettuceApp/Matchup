-- +goose Up
ALTER TABLE public.matchups ADD COLUMN IF NOT EXISTS likes_count integer NOT NULL DEFAULT 0;
ALTER TABLE public.matchups ADD COLUMN IF NOT EXISTS comments_count integer NOT NULL DEFAULT 0;
ALTER TABLE public.brackets ADD COLUMN IF NOT EXISTS likes_count integer NOT NULL DEFAULT 0;
ALTER TABLE public.brackets ADD COLUMN IF NOT EXISTS comments_count integer NOT NULL DEFAULT 0;

-- Backfill the new counter columns from the source-of-truth tables.
UPDATE public.matchups m SET likes_count = (SELECT COUNT(*) FROM public.likes WHERE matchup_id = m.id);
UPDATE public.matchups m SET comments_count = (SELECT COUNT(*) FROM public.comments WHERE matchup_id = m.id);
UPDATE public.brackets b SET likes_count = (SELECT COUNT(*) FROM public.bracket_likes WHERE bracket_id = b.id);
UPDATE public.brackets b SET comments_count = (SELECT COUNT(*) FROM public.bracket_comments WHERE bracket_id = b.id);

-- ---------- triggers ----------

-- matchup likes
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.bump_matchup_likes_count() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.matchups SET likes_count = likes_count + 1 WHERE id = NEW.matchup_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE public.matchups SET likes_count = GREATEST(likes_count - 1, 0) WHERE id = OLD.matchup_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_bump_matchup_likes_count ON public.likes;
CREATE TRIGGER trg_bump_matchup_likes_count
AFTER INSERT OR DELETE ON public.likes
FOR EACH ROW EXECUTE FUNCTION public.bump_matchup_likes_count();

-- matchup comments
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.bump_matchup_comments_count() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.matchups SET comments_count = comments_count + 1 WHERE id = NEW.matchup_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE public.matchups SET comments_count = GREATEST(comments_count - 1, 0) WHERE id = OLD.matchup_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_bump_matchup_comments_count ON public.comments;
CREATE TRIGGER trg_bump_matchup_comments_count
AFTER INSERT OR DELETE ON public.comments
FOR EACH ROW EXECUTE FUNCTION public.bump_matchup_comments_count();

-- bracket likes
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.bump_bracket_likes_count() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.brackets SET likes_count = likes_count + 1 WHERE id = NEW.bracket_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE public.brackets SET likes_count = GREATEST(likes_count - 1, 0) WHERE id = OLD.bracket_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_bump_bracket_likes_count ON public.bracket_likes;
CREATE TRIGGER trg_bump_bracket_likes_count
AFTER INSERT OR DELETE ON public.bracket_likes
FOR EACH ROW EXECUTE FUNCTION public.bump_bracket_likes_count();

-- bracket comments
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION public.bump_bracket_comments_count() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'INSERT' THEN
    UPDATE public.brackets SET comments_count = comments_count + 1 WHERE id = NEW.bracket_id;
  ELSIF TG_OP = 'DELETE' THEN
    UPDATE public.brackets SET comments_count = GREATEST(comments_count - 1, 0) WHERE id = OLD.bracket_id;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_bump_bracket_comments_count ON public.bracket_comments;
CREATE TRIGGER trg_bump_bracket_comments_count
AFTER INSERT OR DELETE ON public.bracket_comments
FOR EACH ROW EXECUTE FUNCTION public.bump_bracket_comments_count();

-- +goose Down
DROP TRIGGER IF EXISTS trg_bump_bracket_comments_count ON public.bracket_comments;
DROP TRIGGER IF EXISTS trg_bump_bracket_likes_count ON public.bracket_likes;
DROP TRIGGER IF EXISTS trg_bump_matchup_comments_count ON public.comments;
DROP TRIGGER IF EXISTS trg_bump_matchup_likes_count ON public.likes;

DROP FUNCTION IF EXISTS public.bump_bracket_comments_count();
DROP FUNCTION IF EXISTS public.bump_bracket_likes_count();
DROP FUNCTION IF EXISTS public.bump_matchup_comments_count();
DROP FUNCTION IF EXISTS public.bump_matchup_likes_count();

ALTER TABLE public.brackets DROP COLUMN IF EXISTS comments_count;
ALTER TABLE public.brackets DROP COLUMN IF EXISTS likes_count;
ALTER TABLE public.matchups DROP COLUMN IF EXISTS comments_count;
ALTER TABLE public.matchups DROP COLUMN IF EXISTS likes_count;
