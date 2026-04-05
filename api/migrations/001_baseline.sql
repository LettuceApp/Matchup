-- +goose Up
-- Baseline schema captured from live database on 2026-03-27.
-- This migration represents the initial state of the Matchup database.

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;

-- =====================
-- TABLES
-- =====================

CREATE TABLE public.users (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    username character varying(255) NOT NULL,
    email character varying(100) NOT NULL,
    password character varying(255) NOT NULL,
    avatar_path character varying(255),
    is_admin boolean DEFAULT false,
    is_private boolean DEFAULT false NOT NULL,
    followers_count bigint DEFAULT 0 NOT NULL,
    following_count bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT users_username_not_empty CHECK (btrim(username::text) <> ''::text)
);

CREATE TABLE public.follows (
    id bigserial PRIMARY KEY,
    follower_id bigint NOT NULL,
    followed_id bigint NOT NULL,
    created_at timestamp with time zone,
    CONSTRAINT follows_no_self_follow CHECK (follower_id <> followed_id)
);

CREATE TABLE public.anonymous_devices (
    id bigserial PRIMARY KEY,
    device_id character varying(36) NOT NULL,
    user_agent_hash text NOT NULL,
    ip_hash text NOT NULL,
    created_at timestamp with time zone,
    last_seen_at timestamp with time zone
);

CREATE TABLE public.matchups (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    title character varying(255) NOT NULL,
    content text NOT NULL,
    author_id bigint NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    status character varying(20) DEFAULT 'published' NOT NULL,
    end_mode character varying(20) DEFAULT 'manual' NOT NULL,
    duration_seconds bigint DEFAULT 0 NOT NULL,
    bracket_id bigint,
    round bigint,
    seed bigint,
    winner_item_id bigint,
    start_time timestamp with time zone,
    end_time timestamp with time zone,
    visibility character varying(20) DEFAULT 'public' NOT NULL
);

CREATE TABLE public.matchup_items (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    matchup_id bigint NOT NULL,
    item text,
    votes bigint
);

CREATE TABLE public.matchup_votes (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint,
    anon_id character varying(36),
    matchup_public_id uuid NOT NULL,
    matchup_item_public_id uuid NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.matchup_vote_rollups (
    matchup_id bigserial PRIMARY KEY,
    legacy_votes bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.brackets (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    author_id bigint NOT NULL,
    size bigint NOT NULL,
    status character varying(20) DEFAULT 'draft' NOT NULL,
    current_round bigint DEFAULT 1,
    visibility character varying(20) DEFAULT 'public' NOT NULL,
    advance_mode character varying(20) DEFAULT 'manual' NOT NULL,
    round_duration_seconds bigint DEFAULT 0 NOT NULL,
    round_started_at timestamp with time zone,
    round_ends_at timestamp with time zone,
    completed_at timestamp with time zone,
    champion_matchup_id bigint,
    champion_item_id bigint,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.bracket_comments (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    bracket_id bigint NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.bracket_likes (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    bracket_id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.comments (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    matchup_id bigint NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE public.likes (
    id bigserial PRIMARY KEY,
    public_id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id bigint NOT NULL,
    matchup_id bigint NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE TABLE public.reset_passwords (
    id bigserial PRIMARY KEY,
    created_at timestamp with time zone,
    updated_at timestamp with time zone,
    deleted_at timestamp with time zone,
    email character varying(100) NOT NULL,
    token character varying(255) NOT NULL
);

-- =====================
-- UNIQUE CONSTRAINTS
-- =====================

ALTER TABLE ONLY public.users ADD CONSTRAINT uni_users_email UNIQUE (email);
ALTER TABLE ONLY public.users ADD CONSTRAINT uni_users_username UNIQUE (username);

-- =====================
-- INDEXES
-- =====================

-- Users
CREATE UNIQUE INDEX idx_users_public_id ON public.users USING btree (public_id);
CREATE INDEX idx_users_username ON public.users USING btree (username);

-- Follows
CREATE UNIQUE INDEX idx_follows_unique ON public.follows USING btree (follower_id, followed_id);
CREATE INDEX idx_follows_follower_id ON public.follows USING btree (follower_id);
CREATE INDEX idx_follows_followed_id ON public.follows USING btree (followed_id);
CREATE INDEX idx_follows_follower_created ON public.follows USING btree (follower_id, created_at);
CREATE INDEX idx_follows_followed_created ON public.follows USING btree (followed_id, created_at);

-- Anonymous Devices
CREATE UNIQUE INDEX idx_anonymous_devices_device_id ON public.anonymous_devices USING btree (device_id);

-- Matchups
CREATE UNIQUE INDEX idx_matchups_public_id ON public.matchups USING btree (public_id);
CREATE INDEX idx_matchups_bracket_id ON public.matchups USING btree (bracket_id);

-- Matchup Items
CREATE UNIQUE INDEX idx_matchup_items_public_id ON public.matchup_items USING btree (public_id);
CREATE INDEX idx_matchup_items_matchup_id ON public.matchup_items USING btree (matchup_id);

-- Matchup Votes
CREATE UNIQUE INDEX idx_matchup_votes_public_id ON public.matchup_votes USING btree (public_id);
CREATE UNIQUE INDEX idx_matchup_vote_user_matchup_public ON public.matchup_votes USING btree (user_id, matchup_public_id);
CREATE UNIQUE INDEX idx_matchup_vote_anon_matchup ON public.matchup_votes USING btree (anon_id, matchup_public_id);
CREATE INDEX idx_matchup_votes_user_id ON public.matchup_votes USING btree (user_id);
CREATE INDEX idx_matchup_votes_anon_id ON public.matchup_votes USING btree (anon_id);
CREATE INDEX idx_matchup_votes_matchup_public_id ON public.matchup_votes USING btree (matchup_public_id);
CREATE INDEX idx_matchup_votes_matchup_item_public_id ON public.matchup_votes USING btree (matchup_item_public_id);

-- Brackets
CREATE UNIQUE INDEX idx_brackets_public_id ON public.brackets USING btree (public_id);
CREATE INDEX idx_brackets_author_id ON public.brackets USING btree (author_id);

-- Bracket Comments
CREATE UNIQUE INDEX idx_bracket_comments_public_id ON public.bracket_comments USING btree (public_id);

-- Bracket Likes
CREATE UNIQUE INDEX idx_bracket_likes_public_id ON public.bracket_likes USING btree (public_id);

-- Comments
CREATE UNIQUE INDEX idx_comments_public_id ON public.comments USING btree (public_id);

-- Likes
CREATE UNIQUE INDEX idx_likes_public_id ON public.likes USING btree (public_id);

-- Reset Passwords
CREATE INDEX idx_reset_passwords_deleted_at ON public.reset_passwords USING btree (deleted_at);

-- =====================
-- FOREIGN KEYS
-- =====================

ALTER TABLE ONLY public.brackets ADD CONSTRAINT fk_brackets_author FOREIGN KEY (author_id) REFERENCES public.users(id) ON UPDATE CASCADE ON DELETE SET NULL;
ALTER TABLE ONLY public.matchups ADD CONSTRAINT fk_matchups_author FOREIGN KEY (author_id) REFERENCES public.users(id) ON UPDATE CASCADE ON DELETE SET NULL;
ALTER TABLE ONLY public.matchup_items ADD CONSTRAINT fk_matchups_items FOREIGN KEY (matchup_id) REFERENCES public.matchups(id) ON DELETE CASCADE;
ALTER TABLE ONLY public.comments ADD CONSTRAINT fk_comments_author FOREIGN KEY (user_id) REFERENCES public.users(id);
ALTER TABLE ONLY public.comments ADD CONSTRAINT fk_matchups_comments FOREIGN KEY (matchup_id) REFERENCES public.matchups(id);
ALTER TABLE ONLY public.bracket_comments ADD CONSTRAINT fk_bracket_comments_author FOREIGN KEY (user_id) REFERENCES public.users(id);

-- +goose Down
DROP TABLE IF EXISTS public.bracket_comments;
DROP TABLE IF EXISTS public.bracket_likes;
DROP TABLE IF EXISTS public.comments;
DROP TABLE IF EXISTS public.likes;
DROP TABLE IF EXISTS public.matchup_votes;
DROP TABLE IF EXISTS public.matchup_vote_rollups;
DROP TABLE IF EXISTS public.matchup_items;
DROP TABLE IF EXISTS public.matchups;
DROP TABLE IF EXISTS public.brackets;
DROP TABLE IF EXISTS public.follows;
DROP TABLE IF EXISTS public.reset_passwords;
DROP TABLE IF EXISTS public.anonymous_devices;
DROP TABLE IF EXISTS public.users;
DROP EXTENSION IF EXISTS pgcrypto;
