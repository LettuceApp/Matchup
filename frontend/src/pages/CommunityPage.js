import React, { useCallback, useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import {
  getCommunityBySlug,
  joinCommunity,
  leaveCommunity,
} from '../services/api';
import '../styles/CommunityPage.css';

/*
 * CommunityPage — Phase 1b placeholder.
 *
 * Just enough surface so post-create redirects don't 404:
 *   - Header with banner / avatar / name / member count
 *   - Description
 *   - Join / Leave button (gated by auth)
 *   - "Coming soon" panel for the feed
 *
 * Phase 1c will replace the placeholder with the real community
 * home: feed of community-scoped matchups + brackets, member list,
 * rules tab, mod tools entry point.
 */
const CommunityPage = () => {
  const { slug } = useParams();
  const [community, setCommunity] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [pendingMembership, setPendingMembership] = useState(false);

  const isAuthed = typeof window !== 'undefined' && Boolean(localStorage.getItem('token'));

  const loadCommunity = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await getCommunityBySlug(slug);
      const data = res?.data?.community ?? res?.data ?? null;
      setCommunity(data);
    } catch (err) {
      const status = err?.response?.status;
      if (status === 404) {
        setError('Community not found.');
      } else {
        setError('Could not load this community right now.');
      }
    } finally {
      setLoading(false);
    }
  }, [slug]);

  useEffect(() => {
    loadCommunity();
  }, [loadCommunity]);

  const handleJoin = async () => {
    if (!community || pendingMembership) return;
    if (!isAuthed) {
      // Anon viewers see a Sign in prompt instead of a no-op button.
      // Skip the network round-trip and route them to log in. After
      // login they can come back and join.
      window.location.href = `/login?next=${encodeURIComponent(`/c/${slug}`)}`;
      return;
    }
    setPendingMembership(true);
    try {
      await joinCommunity(community.id);
      await loadCommunity();
    } catch (err) {
      console.warn('Join failed', err);
    } finally {
      setPendingMembership(false);
    }
  };

  const handleLeave = async () => {
    if (!community || pendingMembership) return;
    setPendingMembership(true);
    try {
      await leaveCommunity(community.id);
      await loadCommunity();
    } catch (err) {
      console.warn('Leave failed', err);
    } finally {
      setPendingMembership(false);
    }
  };

  if (loading) {
    return (
      <div className="community-page community-page--loading">
        <div className="community-page__spinner" aria-label="Loading community" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="community-page community-page--error">
        <h1>{error}</h1>
        <Link to="/home" className="community-page__home-link">
          ← Back to home
        </Link>
      </div>
    );
  }

  if (!community) return null;

  const viewerRole = community.viewer_role || '';
  const isMember = viewerRole === 'member' || viewerRole === 'mod' || viewerRole === 'owner';
  const isBanned = viewerRole === 'banned';

  return (
    <div className="community-page">
      {/* Banner. Empty for newly-created communities until banner
          upload lands in a follow-up. */}
      <div className="community-page__banner" aria-hidden="true">
        {community.banner_path && (
          <img src={community.banner_path} alt="" />
        )}
      </div>

      <header className="community-page__header">
        <div className="community-page__identity">
          <div className="community-page__avatar">
            {community.avatar_path ? (
              <img src={community.avatar_path} alt="" />
            ) : (
              <span aria-hidden="true">{community.name.charAt(0).toUpperCase()}</span>
            )}
          </div>
          <div className="community-page__title-block">
            <h1 className="community-page__name">{community.name}</h1>
            <p className="community-page__slug">/c/{community.slug}</p>
            <p className="community-page__meta">
              <strong>{community.member_count}</strong>{' '}
              {community.member_count === 1 ? 'member' : 'members'}
              {community.owner_username && (
                <>
                  {' · '}Owner{' '}
                  <Link to={`/users/${community.owner_username}`}>
                    @{community.owner_username}
                  </Link>
                </>
              )}
            </p>
          </div>
        </div>

        <div className="community-page__actions">
          {isBanned ? (
            <button type="button" className="community-page__btn" disabled>
              Banned
            </button>
          ) : isMember && viewerRole !== 'owner' ? (
            <button
              type="button"
              className="community-page__btn community-page__btn--ghost"
              onClick={handleLeave}
              disabled={pendingMembership}
            >
              {pendingMembership ? 'Leaving…' : 'Leave'}
            </button>
          ) : viewerRole === 'owner' ? (
            <Link to={`/c/${slug}/settings`} className="community-page__btn community-page__btn--ghost">
              Settings
            </Link>
          ) : (
            <button
              type="button"
              className="community-page__btn community-page__btn--primary"
              onClick={handleJoin}
              disabled={pendingMembership}
            >
              {pendingMembership ? 'Joining…' : 'Join community'}
            </button>
          )}
        </div>
      </header>

      {community.description && (
        <section className="community-page__about">
          <p>{community.description}</p>
        </section>
      )}

      {community.tags?.length > 0 && (
        <div className="community-page__tags">
          {community.tags.map((tag) => (
            <span key={tag} className="community-page__tag">
              {tag}
            </span>
          ))}
        </div>
      )}

      <section className="community-page__feed-placeholder">
        <h2>Community feed</h2>
        <p>
          Community-scoped matchups and brackets will show up here. We're
          shipping that piece next.
        </p>
      </section>
    </div>
  );
};

export default CommunityPage;
