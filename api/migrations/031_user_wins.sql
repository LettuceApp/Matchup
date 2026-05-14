-- +goose Up
-- 031_user_wins.sql — wins tracking for the social-loop cycle.
--
-- When a matchup is decided (CompleteMatchup, OverrideMatchupWinner,
-- bracket-round auto-advance) AND the winning item references a user
-- (matchup_items.user_id IS NOT NULL — migration 030), increment two
-- counters in the same transaction:
--   1. users.wins_count — global tally surfaced as a stat tile on
--      the user's profile page.
--   2. community_member_wins.wins_count — per-(community, user)
--      tally that backs the Champions leaderboard on the community
--      page. Only incremented when the matchup is community-scoped.
--
-- Two separate counters (not "one global + filter") because the
-- Champions read path is hot (rendered on the community Champions
-- tab + a future sidebar widget) and needs to ORDER BY wins_count
-- without scanning matchups. The trade-off is one extra UPDATE per
-- community-scoped win.
--
-- Schema choice — community_member_wins is its own table rather than
-- a column on community_memberships because (a) we want a row even
-- for non-members who later joined (rare but possible) and (b)
-- last_won_at is a leaderboard tiebreaker that doesn't belong on the
-- membership row.

ALTER TABLE public.users
  ADD COLUMN wins_count BIGINT NOT NULL DEFAULT 0;

CREATE TABLE public.community_member_wins (
    community_id BIGINT NOT NULL REFERENCES public.communities(id) ON DELETE CASCADE,
    user_id      BIGINT NOT NULL REFERENCES public.users(id)       ON DELETE CASCADE,
    wins_count   BIGINT NOT NULL DEFAULT 0,
    last_won_at  TIMESTAMPTZ NULL,
    PRIMARY KEY (community_id, user_id)
);

-- Leaderboard read path: ORDER BY wins_count DESC, last_won_at DESC.
-- Index covers the WHERE community_id = ? prefix + the sort key.
CREATE INDEX idx_community_member_wins_leaderboard
    ON public.community_member_wins (community_id, wins_count DESC, last_won_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_community_member_wins_leaderboard;
DROP TABLE IF EXISTS public.community_member_wins;
ALTER TABLE public.users
  DROP COLUMN IF EXISTS wins_count;
