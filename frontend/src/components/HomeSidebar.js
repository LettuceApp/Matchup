import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import {
  listMyCommunities,
  logout as serverLogout,
  signOutLocally,
} from '../services/api';
import { CATEGORIES } from '../utils/categories';

// How many categories to show in the collapsed state. With 14 total
// categories the full list pushes "Other" off-screen at standard
// laptop heights — research feedback flagged the cliff. Six gives
// the most-popular categories breathing room and keeps an obvious
// "Show more" affordance for the rest.
const COLLAPSED_CATEGORY_COUNT = 6;

const HomeSidebar = ({ sortMode, onSortChange, categoryFilter, onCategoryChange, mobileOpen = false }) => {
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const isAuthed = Boolean(localStorage.getItem('token'));

  // Progressive-disclosure state for the categories list. "All
  // Categories" stays pinned at the top, the first N body categories
  // stay visible, and the remainder collapses behind a "Show more"
  // toggle. If the active filter is one of the collapsed categories
  // we force-expand so the active highlight is reachable.
  const [categoriesExpanded, setCategoriesExpanded] = useState(false);
  // Communities section collapses by default so the Account section
  // (avatar + Logout) stays visible without scrolling at standard
  // laptop heights. Expanding reveals the full joined list +
  // "+ Create community" CTA. Stays per-mount — re-renders or
  // re-mounts reset to collapsed (the brief specifies "collapsed on
  // initial render").
  const [communitiesExpanded, setCommunitiesExpanded] = useState(false);
  const allCategoriesEntry = CATEGORIES[0];
  const bodyCategories = CATEGORIES.slice(1);
  const topBodyCategories = bodyCategories.slice(0, COLLAPSED_CATEGORY_COUNT);
  const hiddenBodyCategories = bodyCategories.slice(COLLAPSED_CATEGORY_COUNT);
  const activeIsHidden = hiddenBodyCategories.includes(categoryFilter);
  const showHidden = categoriesExpanded || activeIsHidden;

  // My-communities sidebar list. Fetched once on mount for authed
  // users; anon viewers get an empty array from the server (no error)
  // so the section just doesn't render. Re-fetched whenever the auth
  // state changes — most often when the user signs in or creates a
  // new community (we listen for a route-driven refresh trigger).
  const [myCommunities, setMyCommunities] = useState([]);
  useEffect(() => {
    if (!isAuthed) {
      setMyCommunities([]);
      return undefined;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await listMyCommunities({ limit: 30 });
        if (cancelled) return;
        setMyCommunities(res?.data?.communities || []);
      } catch (err) {
        if (cancelled) return;
        console.warn('listMyCommunities failed', err);
      }
    })();
    return () => { cancelled = true; };
  }, [isAuthed]);

  const handleLogout = async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    if (refreshToken) {
      try { await serverLogout(refreshToken); } catch { /* best-effort */ }
    }
    signOutLocally();
    navigate('/login', { replace: true });
  };

  const profilePath = username && username !== 'undefined'
    ? `/users/${username}`
    : userId ? `/users/${userId}` : null;

  return (
    <aside className={`home-sidebar${mobileOpen ? ' home-sidebar--mobile-open' : ''}`}>
      <div className="home-sidebar__brand" onClick={() => navigate('/home')}>
        Matchup
      </div>

      <nav className="home-sidebar__nav">
        <div className="home-sidebar__section-label">Sort By</div>
        {[
          { key: 'latest', label: 'Latest' },
          { key: 'trending', label: 'Trending' },
          { key: 'most-played', label: 'Most Played' },
        ].map(({ key, label }) => (
          <button
            key={key}
            type="button"
            className={`home-sidebar__nav-item${sortMode === key ? ' home-sidebar__nav-item--active' : ''}`}
            onClick={() => onSortChange(key)}
          >
            {label}
          </button>
        ))}

        <div className="home-sidebar__section-label">Categories</div>
        {[allCategoriesEntry, ...topBodyCategories].map((cat) => {
          const value = cat === 'All Categories' ? 'all' : cat;
          const isActive = categoryFilter === value;
          return (
            <button
              key={cat}
              type="button"
              className={`home-sidebar__nav-item${isActive ? ' home-sidebar__nav-item--active' : ''}`}
              onClick={() => onCategoryChange(value)}
            >
              {cat}
            </button>
          );
        })}
        {showHidden && hiddenBodyCategories.map((cat) => {
          const isActive = categoryFilter === cat;
          return (
            <button
              key={cat}
              type="button"
              className={`home-sidebar__nav-item${isActive ? ' home-sidebar__nav-item--active' : ''}`}
              onClick={() => onCategoryChange(cat)}
            >
              {cat}
            </button>
          );
        })}
        {hiddenBodyCategories.length > 0 && !activeIsHidden && (
          <button
            type="button"
            className="home-sidebar__nav-item home-sidebar__nav-item--muted home-sidebar__nav-item--toggle"
            onClick={() => setCategoriesExpanded((v) => !v)}
            aria-expanded={categoriesExpanded}
          >
            {categoriesExpanded ? 'Show less' : `Show ${hiddenBodyCategories.length} more`}
          </button>
        )}

        {/* Communities section. Lists the user's joined communities
            with a role badge (owner / mod / member) so they can jump
            into any of them from the sidebar. "+ Create community"
            stays pinned at the bottom of the section as the
            recurrent CTA. Collapsed-by-default per the home-cleanup
            brief — Communities can grow long enough to push the
            Account section below the fold, so we hide them behind a
            disclosure toggle and let the user opt in. */}
        {isAuthed && (
          <>
            <button
              type="button"
              className="home-sidebar__section-toggle"
              aria-expanded={communitiesExpanded}
              onClick={() => setCommunitiesExpanded((v) => !v)}
            >
              <span>Communities</span>
              <span aria-hidden="true" className="home-sidebar__section-toggle-caret">
                {communitiesExpanded ? '▾' : '▸'}
              </span>
            </button>
            {communitiesExpanded && (
              <>
                {myCommunities.map((c) => {
                  const role = c.viewer_role || 'member';
                  const initial = (c.name || '?').charAt(0).toUpperCase();
                  return (
                    <button
                      key={c.id}
                      type="button"
                      className="home-sidebar__community-row"
                      onClick={() => navigate(`/c/${c.slug}`)}
                      title={`${c.name} · ${role}`}
                    >
                      <span className="home-sidebar__community-avatar" aria-hidden="true">
                        {c.avatar_path ? (
                          <img src={c.avatar_path} alt="" />
                        ) : (
                          <span>{initial}</span>
                        )}
                      </span>
                      <span className="home-sidebar__community-name">{c.name}</span>
                      <span
                        className={`home-sidebar__community-role home-sidebar__community-role--${role}`}
                        aria-label={`Your role: ${role}`}
                      >
                        {role === 'owner' ? '👑' : role === 'mod' ? '⚔' : '·'}
                      </span>
                    </button>
                  );
                })}
                <button
                  type="button"
                  className="home-sidebar__nav-item home-sidebar__nav-item--muted"
                  onClick={() => navigate('/communities/new')}
                >
                  + Create community
                </button>
              </>
            )}
          </>
        )}

        <div className="home-sidebar__section-label">Account</div>
        {isAuthed ? (
          <>
            {profilePath && (
              <button
                type="button"
                className="home-sidebar__account-row"
                onClick={() => navigate(profilePath)}
              >
                {userId && <ProfilePic userId={userId} size={32} />}
                <span>{username && username !== 'undefined' ? username : 'Profile'}</span>
              </button>
            )}
            <button
              type="button"
              className="home-sidebar__nav-item home-sidebar__nav-item--muted"
              onClick={handleLogout}
            >
              Logout
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className="home-sidebar__nav-item"
              onClick={() => navigate('/login')}
            >
              Sign in
            </button>
            <button
              type="button"
              className="home-sidebar__nav-item"
              onClick={() => navigate('/register')}
            >
              Sign up
            </button>
          </>
        )}
      </nav>
    </aside>
  );
};

export default HomeSidebar;
