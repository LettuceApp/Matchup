-- +goose Up
-- Communities v1 — schema foundation.
--
-- A community is a Reddit-style group: name, slug, description, banner,
-- tags. Users join, an owner runs it, mods enforce rules, content
-- (matchups + brackets) can be community-scoped or remain standalone.
--
-- Privacy roadmap: column accepts 'public' | 'restricted' | 'private'
-- but v1 only creates 'public' communities. The schema is future-proof
-- so we don't need a migration to enable restricted/private later.
--
-- Role enum: 'owner' | 'mod' | 'member' | 'banned'. The "admin" tier
-- was intentionally cut from v1 to keep the role matrix simple; can
-- be added without a schema change because role is a text column.
--
-- Denormalised counts (member_count, matchup_count, bracket_count)
-- live on the parent row so community cards in directories don't fan
-- out to N COUNT(*) queries. They're updated by the relevant write
-- paths (join/leave, create-matchup-in-community, etc).
--
-- Soft delete via deleted_at — recoverable for 30 days by the same
-- daily cron that handles user deletes. Hard cascade on memberships /
-- rules / matchups.community_id / brackets.community_id once the
-- community is hard-deleted.

CREATE TABLE public.communities (
    id              bigserial   PRIMARY KEY,
    public_id       text        NOT NULL UNIQUE,
    slug            text        NOT NULL UNIQUE,
    name            text        NOT NULL,
    description     text        NOT NULL DEFAULT '',
    avatar_path     text        NOT NULL DEFAULT '',
    banner_path     text        NOT NULL DEFAULT '',
    tags            text[]      NOT NULL DEFAULT '{}'::text[],
    privacy         text        NOT NULL DEFAULT 'public',
    owner_id        bigint      NOT NULL REFERENCES public.users(id) ON DELETE RESTRICT,
    member_count    integer     NOT NULL DEFAULT 0,
    matchup_count   integer     NOT NULL DEFAULT 0,
    bracket_count   integer     NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT NOW(),
    updated_at      timestamptz NOT NULL DEFAULT NOW(),
    deleted_at      timestamptz NULL,
    CONSTRAINT communities_privacy_chk CHECK (privacy IN ('public', 'restricted', 'private'))
);
CREATE INDEX idx_communities_owner          ON public.communities (owner_id);
CREATE INDEX idx_communities_deleted_at     ON public.communities (deleted_at);
-- GIN on tags so "communities with tag X" stays cheap as the tags
-- array grows. Same shape used for matchup tags.
CREATE INDEX idx_communities_tags           ON public.communities USING GIN (tags);

CREATE TABLE public.community_memberships (
    id                  bigserial   PRIMARY KEY,
    community_id        bigint      NOT NULL REFERENCES public.communities(id) ON DELETE CASCADE,
    user_id             bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    role                text        NOT NULL DEFAULT 'member',
    joined_at           timestamptz NOT NULL DEFAULT NOW(),
    invited_by_user_id  bigint      NULL REFERENCES public.users(id) ON DELETE SET NULL,
    banned_at           timestamptz NULL,
    banned_by_user_id   bigint      NULL REFERENCES public.users(id) ON DELETE SET NULL,
    banned_reason       text        NULL,
    CONSTRAINT community_memberships_role_chk CHECK (role IN ('owner', 'mod', 'member', 'banned')),
    -- One membership row per (community, user). Banned users still
    -- have a row — role flips to 'banned' rather than DELETE-ing so
    -- a re-join attempt is rejected by the unique constraint.
    CONSTRAINT community_memberships_uniq UNIQUE (community_id, user_id)
);
CREATE INDEX idx_community_memberships_user
    ON public.community_memberships (user_id);
CREATE INDEX idx_community_memberships_role
    ON public.community_memberships (community_id, role);

CREATE TABLE public.community_rules (
    id              bigserial   PRIMARY KEY,
    community_id    bigint      NOT NULL REFERENCES public.communities(id) ON DELETE CASCADE,
    position        integer     NOT NULL DEFAULT 0,
    title           text        NOT NULL,
    body            text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT NOW(),
    updated_at      timestamptz NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_community_rules_community
    ON public.community_rules (community_id, position);

-- Attach community_id to matchups + brackets. Nullable so standalone
-- mode (no community) keeps working unchanged; that's the existing
-- behavior. SET NULL on community delete so a deleted community
-- doesn't take the matchup with it — the matchup just becomes
-- standalone again.
ALTER TABLE public.matchups
    ADD COLUMN community_id bigint NULL
        REFERENCES public.communities(id) ON DELETE SET NULL;
CREATE INDEX idx_matchups_community
    ON public.matchups (community_id) WHERE community_id IS NOT NULL;

ALTER TABLE public.brackets
    ADD COLUMN community_id bigint NULL
        REFERENCES public.communities(id) ON DELETE SET NULL;
CREATE INDEX idx_brackets_community
    ON public.brackets (community_id) WHERE community_id IS NOT NULL;

-- +goose Down
ALTER TABLE public.brackets DROP COLUMN IF EXISTS community_id;
ALTER TABLE public.matchups DROP COLUMN IF EXISTS community_id;
DROP TABLE IF EXISTS public.community_rules;
DROP TABLE IF EXISTS public.community_memberships;
DROP TABLE IF EXISTS public.communities;
