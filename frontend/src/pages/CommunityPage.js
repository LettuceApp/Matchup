import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import HomeCard from '../components/HomeCard';
import {
  getCommunityBySlug,
  getCommunityFeed,
  joinCommunity,
  leaveCommunity,
} from '../services/api';
import {
  accentForSlug,
  accentHoverForSlug,
  gradientForSlug,
} from '../utils/communityGradients';
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

  // Feed state — matchups + brackets the community owns. Loaded
  // separately from the community itself so the header can render
  // while the feed is still in flight.
  const [feedMatchups, setFeedMatchups] = useState([]);
  const [feedBrackets, setFeedBrackets] = useState([]);
  const [feedLoading, setFeedLoading] = useState(false);
  const [feedError, setFeedError] = useState(null);

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

  // Push the community's chosen gradient up to :root so global
  // elements (NavigationBar brand wordmark, feed-card stripes, etc.)
  // repaint with the community's theme. The accent + hover + soft
  // vars are extracted from the gradient's mid + dark stops so
  // single-color UI (buttons, owner @-links) can consume them
  // without re-deriving them at render time.
  //
  // The community page follows the app-wide theme (dark by default)
  // like every other page — no forced light-mode override. An earlier
  // cycle pinned the page to a light shell regardless of the viewer's
  // theme; reverted at the user's request because pages flipping
  // modes mid-navigation felt jarring. The accent var work stays
  // because it's the brand-theming hook the rest of the page uses.
  //
  // Cleanup removes the vars on unmount so the brand wordmark
  // reverts to stardust off-community.
  useEffect(() => {
    if (!community) return undefined;
    const slug = community.theme_gradient || '';
    const themeGradient = gradientForSlug(slug);
    const accent = accentForSlug(slug);
    const accentHover = accentHoverForSlug(slug);
    const root = document.documentElement;
    root.style.setProperty('--page-accent-gradient', themeGradient);
    root.style.setProperty('--c-accent', accent);
    root.style.setProperty('--c-accent-hover', accentHover);
    root.style.setProperty(
      '--c-accent-soft',
      `color-mix(in srgb, ${accent} 16%, transparent)`,
    );
    return () => {
      root.style.removeProperty('--page-accent-gradient');
      root.style.removeProperty('--c-accent');
      root.style.removeProperty('--c-accent-hover');
      root.style.removeProperty('--c-accent-soft');
    };
  }, [community]);

  // Fetch the community feed once we know the community's public_id.
  // Re-runs only on community-change (new slug, or first load) — not
  // on every membership state change.
  useEffect(() => {
    if (!community?.id) return;
    let cancelled = false;
    setFeedLoading(true);
    setFeedError(null);
    (async () => {
      try {
        const res = await getCommunityFeed(community.id, { limit: 20 });
        if (cancelled) return;
        setFeedMatchups(res?.data?.matchups || []);
        setFeedBrackets(res?.data?.brackets || []);
      } catch (err) {
        if (cancelled) return;
        console.warn('Community feed load failed', err);
        setFeedError('Could not load the community feed.');
      } finally {
        if (!cancelled) setFeedLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [community?.id]);

  // Interleave matchups + brackets by created_at DESC for the
  // unified feed render. Cheap O(n+m) merge since each list already
  // arrives sorted by created_at from the backend.
  const feedItems = useMemo(() => {
    const combined = [
      ...feedMatchups.map((m) => ({ ...m, _kind: 'matchup' })),
      ...feedBrackets.map((b) => ({ ...b, _kind: 'bracket' })),
    ];
    combined.sort((a, b) => {
      const ta = new Date(a.created_at).getTime();
      const tb = new Date(b.created_at).getTime();
      return tb - ta;
    });
    return combined;
  }, [feedMatchups, feedBrackets]);

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

  // Theme-aware backgrounds. The owner's chosen gradient slug fills
  // the banner whenever no banner image is uploaded, and tints the
  // initial-letter avatar fallback when no avatar image is set.
  // Unknown / empty slug → default stardust palette so the page
  // never renders a flat dead surface.
  const themeGradient = gradientForSlug(community.theme_gradient || '');

  return (
    <div className="community-page">
      {/* Banner. When the owner has uploaded a banner image it wins;
          otherwise the chosen theme gradient fills the strip so a
          freshly-created community already feels themed. */}
      <div
        className="community-page__banner"
        aria-hidden="true"
        style={community.banner_path ? undefined : { background: themeGradient }}
      >
        {community.banner_path && (
          <img src={community.banner_path} alt="" />
        )}
      </div>

      <header className="community-page__header">
        <div className="community-page__identity">
          <div
            className="community-page__avatar"
            style={community.avatar_path ? undefined : { background: themeGradient }}
          >
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
              <Link to={`/c/${slug}/members`} className="community-page__members-link">
                <strong>{community.member_count}</strong>{' '}
                {community.member_count === 1 ? 'member' : 'members'}
              </Link>
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
            <Link
              to={`/c/${slug}/settings`}
              className="community-page__btn community-page__btn--ghost community-page__btn--settings"
            >
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

      {/* About section. Hidden when description is empty / whitespace
          / case-insensitively equal to the community name — old
          communities had description == name as a placeholder, which
          made the section look redundant alongside the title. */}
      {(() => {
        const desc = (community.description || '').trim();
        const name = (community.name || '').trim().toLowerCase();
        const showAbout = desc.length > 0 && desc.toLowerCase() !== name;
        return showAbout ? (
          <section className="community-page__about">
            <p>{community.description}</p>
          </section>
        ) : null;
      })()}

      {/* Tags. Filter out anything that case-insensitively matches
          the community name OR the description's first word — those
          tags duplicate info that's already in the header / about
          section and just clutter the chip row. */}
      {(() => {
        const name = (community.name || '').trim().toLowerCase();
        const desc = (community.description || '').trim().toLowerCase();
        const filtered = (community.tags || []).filter((t) => {
          const tag = (t || '').trim().toLowerCase();
          if (!tag) return false;
          if (tag === name) return false;
          if (tag === desc) return false;
          return true;
        });
        if (filtered.length === 0) return null;
        return (
          <div className="community-page__tags">
            {filtered.map((tag) => (
              <span key={tag} className="community-page__tag">
                {tag}
              </span>
            ))}
          </div>
        );
      })()}

      {/* Member-only CTAs. Owners and mods also count as members for
          create purposes (the backend's permission gate accepts
          member / mod / owner). Banned users see nothing here — same
          posture as the Join button area. */}
      {isMember && (
        <div className="community-page__create-row">
          <Link
            to={`/users/${community.owner_username || community.id}/create-matchup?community=${community.id}`}
            className="community-page__btn community-page__btn--primary"
          >
            + New matchup
          </Link>
          <Link
            to={`/brackets/new?community=${community.id}`}
            className="community-page__btn community-page__btn--ghost"
          >
            + New bracket
          </Link>
        </div>
      )}

      {/* The chosen theme gradient is exposed as a CSS custom
          property on the feed container so each HomeCard inside
          picks up a 4px gradient stripe at its top (see the
          `.community-page__feed .home-card::before` rule in
          CommunityPage.css). Scoped here intentionally — HomeCard
          is reused across home / profile / search, and we only
          want the tint when the card is rendered inside a themed
          community feed. */}
      <section
        className="community-page__feed"
        style={{ '--community-accent-gradient': themeGradient }}
      >
        <h2 className="community-page__feed-title">Community feed</h2>
        {feedLoading && feedItems.length === 0 && (
          <p className="community-page__feed-empty">Loading…</p>
        )}
        {feedError && (
          <p className="community-page__feed-empty community-page__feed-empty--error">
            {feedError}
          </p>
        )}
        {!feedLoading && !feedError && feedItems.length === 0 && (
          <p className="community-page__feed-empty">
            No matchups or brackets yet. {isMember ? 'Be the first to start one!' : 'Join to create the first one.'}
          </p>
        )}
        {feedItems.length > 0 && (
          <div className="community-page__feed-grid">
            {feedItems.map((item) => (
              <HomeCard
                key={`${item._kind}-${item.id}`}
                item={item}
                type={item._kind}
                // Replace the deterministic violet/blue HomeCard
                // placeholder gradient with the community's chosen
                // theme — so cards-without-images visibly belong to
                // the themed community instead of falling through to
                // the global app palette. Cards WITH images are
                // unaffected (the image still wins inside the media
                // div). HomeCards rendered on home / profile / search
                // don't pass this prop, so this is strictly opt-in.
                fallbackGradient={themeGradient}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
};

export default CommunityPage;
