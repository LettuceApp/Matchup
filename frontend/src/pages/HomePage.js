import React, { useState, useEffect, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { FiMenu } from 'react-icons/fi';
import {
  getHomeSummary,
  getMatchups,
  getUserLikes,
  getUserBracketLikes,
  logout as serverLogout,
  signOutLocally,
} from '../services/api';
import HomeSidebar from '../components/HomeSidebar';
import HomeCard, { deriveTags } from '../components/HomeCard';
import NotificationBell from '../components/NotificationBell';
import ProfilePic from '../components/ProfilePic';
import ThemeToggleItem from '../components/ThemeToggleItem';
import CreateMenu from '../components/CreateMenu';
import { track } from '../utils/analytics';
import '../styles/HomePage.css';
import '../components/NavigationBar.css';

const HomePage = () => {
  const [sortMode, setSortMode] = useState('latest');
  const [typeFilter, setTypeFilter] = useState('all');
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  // Mobile sidebar drawer state. Below 768px the sidebar lives off-
  // canvas; this flag slides it in. Closing on a category/sort tap
  // keeps the drawer from staying open over the content the user
  // just filtered to.
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [loading, setLoading] = useState(true);
  // Per-viewer "what have I already liked" sets. Fetched once on
  // mount (when authed) so each HomeCard can render its initial heart
  // state without an N+1 round-trip per card. Anon viewers stay with
  // empty sets — the like buttons short-circuit to /login on click.
  const [likedMatchupIds, setLikedMatchupIds] = useState(() => new Set());
  const [likedBracketIds, setLikedBracketIds] = useState(() => new Set());

  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');

  // Top-of-funnel marker for anon visitors. Fires once on first
  // mount per session so we can measure the anon→signup funnel
  // starting from "saw the home page" rather than "site visit"
  // (autocapture covers raw pageview already).
  useEffect(() => {
    if (!userId) {
      track('anon_home_viewed');
    }
    // Empty deps — fire once per mount, not per filter change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // One-shot user-likes fetch. Runs in parallel; either failing is
  // non-fatal (the heart will just render in the unlit state and the
  // first click will reconcile via optimistic update + server call).
  // Authentication-gated — anon users skip both calls entirely.
  useEffect(() => {
    if (!userId) return;
    let cancelled = false;
    Promise.allSettled([
      getUserLikes(userId),
      getUserBracketLikes(userId),
    ]).then(([matchupsRes, bracketsRes]) => {
      if (cancelled) return;
      if (matchupsRes.status === 'fulfilled') {
        const data = matchupsRes.value?.data;
        const list = data?.likes ?? data?.response ?? data ?? [];
        const ids = new Set(
          (Array.isArray(list) ? list : []).map((l) => String(l.matchup_id ?? l.matchupId ?? l.id ?? '')),
        );
        setLikedMatchupIds(ids);
      }
      if (bracketsRes.status === 'fulfilled') {
        const data = bracketsRes.value?.data;
        const list = data?.likes ?? data?.response ?? data ?? [];
        const ids = new Set(
          (Array.isArray(list) ? list : []).map((l) => String(l.bracket_id ?? l.bracketId ?? l.id ?? '')),
        );
        setLikedBracketIds(ids);
      }
    });
    return () => { cancelled = true; };
  }, [userId]);

  useEffect(() => {
    const fetchData = async () => {
      setLoading(true);
      try {
        // Always fetch brackets from home summary (the only public bracket list endpoint)
        const summaryRes = await getHomeSummary(userId);
        const summaryData = summaryRes.data?.summary ?? summaryRes.data?.response ?? summaryRes.data ?? {};
        const homeBrackets = Array.isArray(summaryData.popular_brackets) ? summaryData.popular_brackets : [];

        if (sortMode === 'latest') {
          const res = await getMatchups(1, 24);
          const payload = res.data?.matchups ?? res.data?.response ?? res.data ?? {};
          setMatchups(Array.isArray(payload) ? payload : Array.isArray(payload.matchups) ? payload.matchups : []);
          setBrackets(homeBrackets);
        } else if (sortMode === 'trending') {
          // Prefer hourly trending data; fall back to all-time popular if empty.
          const trending = Array.isArray(summaryData.trending_matchups) && summaryData.trending_matchups.length > 0
            ? summaryData.trending_matchups
            : Array.isArray(summaryData.popular_matchups) ? summaryData.popular_matchups : [];
          setMatchups(trending);
          setBrackets(homeBrackets);
        } else {
          // most-played: matchups with the most votes in the past hour
          const mostPlayed = Array.isArray(summaryData.most_played_matchups) && summaryData.most_played_matchups.length > 0
            ? summaryData.most_played_matchups
            : Array.isArray(summaryData.popular_matchups) ? summaryData.popular_matchups : [];
          setMatchups(mostPlayed);
          setBrackets(homeBrackets);
        }
      } catch (err) {
        console.error('HomePage fetch error:', err);
        setMatchups([]);
        setBrackets([]);
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, [sortMode, userId]);

  // Build combined item list with type tag
  const allItems = useMemo(() => {
    const m = matchups.map((item) => ({ ...item, _type: 'matchup' }));
    const b = brackets.map((item) => ({ ...item, _type: 'bracket' }));
    if (typeFilter === 'matchup') return m;
    if (typeFilter === 'bracket') return b;
    // interleave matchups and brackets
    const out = [];
    const maxLen = Math.max(m.length, b.length);
    for (let i = 0; i < maxLen; i++) {
      if (i < m.length) out.push(m[i]);
      if (i < b.length) out.push(b[i]);
    }
    return out;
  }, [matchups, brackets, typeFilter]);

  // Client-side search + category filter
  const visibleItems = useMemo(() => {
    let items = allItems;
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      items = items.filter((item) => (item.title ?? '').toLowerCase().includes(q));
    }
    if (categoryFilter !== 'all') {
      items = items.filter((item) => {
        const backendTags = Array.isArray(item.tags) && item.tags.length > 0 ? item.tags : null;
        const tags = backendTags ?? deriveTags(item.title ?? '');
        return tags.includes(categoryFilter);
      });
    }
    return items;
  }, [allItems, searchQuery, categoryFilter]);

  const navigateToCreate = () => {
    if (!userId) { navigate('/login'); return; }
    navigate(`/users/${username || userId}/create-matchup`);
  };

  const navigateToCreateBracket = () => {
    if (!userId) { navigate('/login'); return; }
    navigate('/brackets/new');
  };

  // Mirrors NavigationBar.handleLogout — best-effort server revoke,
  // always clear local auth + redirect to /login. Inlined here
  // because the home-topbar now hosts the nav actions itself instead
  // of mounting <NavigationBar />.
  const isAuthed = Boolean(localStorage.getItem('token'));
  const handleLogout = async () => {
    const refreshToken = localStorage.getItem('refresh_token');
    if (refreshToken) {
      try { await serverLogout(refreshToken); } catch { /* ignore */ }
    }
    signOutLocally();
    navigate('/login', { replace: true });
  };

  // Profile-pic dropdown menu. Avatar trigger; menu collapses Home /
  // Admin / Logout / View-profile so the topbar reads clean. Bell stays
  // outside as an unread-state indicator. Outside-click + Escape close.
  const [profileMenuOpen, setProfileMenuOpen] = useState(false);
  const profileMenuRef = useRef(null);
  useEffect(() => {
    if (!profileMenuOpen) return undefined;
    const onPointerDown = (e) => {
      if (profileMenuRef.current && !profileMenuRef.current.contains(e.target)) {
        setProfileMenuOpen(false);
      }
    };
    const onKey = (e) => { if (e.key === 'Escape') setProfileMenuOpen(false); };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [profileMenuOpen]);

  const profileSlug = (username && username !== 'undefined') ? username : userId;
  const goAndClose = (path) => () => {
    setProfileMenuOpen(false);
    navigate(path);
  };
  const handleLogoutFromMenu = async () => {
    setProfileMenuOpen(false);
    await handleLogout();
  };

  // Filter taps from inside the mobile drawer should close it — the
  // user expects the result to be visible, not blocked by the drawer
  // they just used.
  const handleSortChangeMobile = (next) => { setSortMode(next); setMobileSidebarOpen(false); };
  const handleCategoryChangeMobile = (next) => { setCategoryFilter(next); setMobileSidebarOpen(false); };

  return (
    <div className="home-page">
      {/* Mobile drawer scrim. Click anywhere outside the sidebar to
          dismiss. Only renders when the drawer is open so it doesn't
          intercept clicks at all desktop sizes. */}
      {mobileSidebarOpen && (
        <div
          className="home-sidebar-scrim"
          role="presentation"
          onClick={() => setMobileSidebarOpen(false)}
        />
      )}
      <HomeSidebar
        sortMode={sortMode}
        onSortChange={handleSortChangeMobile}
        categoryFilter={categoryFilter}
        onCategoryChange={handleCategoryChangeMobile}
        mobileOpen={mobileSidebarOpen}
      />

      <div className="home-main">
        <div className="home-topbar">
          {/* Hamburger — opens the off-canvas sidebar on small screens.
              Hidden via CSS at desktop sizes so the topbar stays clean. */}
          <button
            type="button"
            className="home-topbar__menu"
            aria-label="Open menu"
            aria-expanded={mobileSidebarOpen}
            onClick={() => setMobileSidebarOpen(true)}
          >
            <FiMenu />
          </button>
          {/* FIND zone. Single search affordance on the page — the
              previous standalone "Search" pill was redundant with this
              input (both escalated to /search). Typing here live-
              filters the visible feed via `searchQuery`; pressing
              Enter (≥2 chars) escalates to the full /search results
              page so users can search beyond what's currently loaded.
              The ⌘K kbd is a visual hint for sighted users; the actual
              shortcut isn't wired yet (when a command palette ships,
              add a document-level keydown listener that opens it). */}
          <div className="home-search-wrap">
            <input
              type="search"
              className="home-search"
              placeholder="Search Matchup…"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  const q = searchQuery.trim();
                  if (q.length >= 2) {
                    e.preventDefault();
                    navigate(`/search?q=${encodeURIComponent(q)}&f=top`);
                  }
                }
              }}
            />
            <kbd className="home-search__hint" aria-hidden="true">⌘K</kbd>
          </div>

          {/* Universal-search launcher. Sits as a peer to the input
              instead of inside it — the input live-filters the
              visible feed as you type, but the magnifier explicitly
              opens the full /search results page (matchups, brackets,
              communities, people). Two distinct intents, two
              affordances: type-here vs. take-me-to-search. The icon
              button is the discoverable "Explore" entry that survives
              even when the user doesn't realise the input itself
              escalates on Enter. */}
          <button
            type="button"
            className="home-topbar__search-launcher"
            aria-label="Open universal search"
            title="Search everything"
            onClick={() => navigate('/search')}
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <circle cx="11" cy="11" r="7" />
              <path d="M21 21l-4.35-4.35" />
            </svg>
          </button>

          {/* CREATE zone — single primary CTA. The previous pair of
              "+ Create Matchup" + "+ Create Bracket" buttons doubled
              up the primary affordance (Hick's Law), so they're now
              folded into a single dropdown. Auth gating + URL
              resolution still happens in the parent handlers so anon
              users keep getting bounced to /login. */}
          <div className="home-topbar__create">
            <CreateMenu
              onMatchup={navigateToCreate}
              onBracket={navigateToCreateBracket}
            />
          </div>

          {/* ACCOUNT zone. Authed users see [bell] [profile▾] with
              Home / Admin / Logout / View profile collapsed into the
              profile dropdown. Bell stays visible because its unread
              count is the whole point — burying it would defeat the
              indicator. Anon users see Sign in / Sign up since there's
              nothing to dropdown into. The CSS adds a border-left +
              padding-left to this zone so Gestalt grouping separates
              identity actions from the Create CTA. */}
          <div className="home-topbar__actions">
            {isAuthed ? (
              <>
                <NotificationBell />
                <div className="home-profile-menu" ref={profileMenuRef}>
                  <button
                    type="button"
                    className="home-profile-menu__trigger"
                    aria-haspopup="menu"
                    aria-expanded={profileMenuOpen}
                    onClick={() => setProfileMenuOpen((v) => !v)}
                  >
                    {userId && <ProfilePic userId={userId} size={44} />}
                  </button>

                  {profileMenuOpen && (
                    <div className="home-profile-menu__panel" role="menu">
                      {profileSlug && (
                        <button
                          type="button"
                          className="home-profile-menu__item"
                          role="menuitem"
                          onClick={goAndClose(`/users/${profileSlug}`)}
                        >
                          View profile
                        </button>
                      )}
                      {localStorage.getItem('isAdmin') === 'true' && (
                        <button
                          type="button"
                          className="home-profile-menu__item"
                          role="menuitem"
                          onClick={goAndClose('/admin')}
                        >
                          Admin
                        </button>
                      )}
                      {/* Theme toggle — same control here as in the
                          NavigationBar avatar menu so the user has a
                          consistent place to switch appearance no
                          matter which page they're on. */}
                      <ThemeToggleItem
                        className="home-profile-menu__item"
                      />
                      <div className="home-profile-menu__divider" />
                      <button
                        type="button"
                        className="home-profile-menu__item home-profile-menu__item--danger"
                        role="menuitem"
                        onClick={handleLogoutFromMenu}
                      >
                        Log out
                      </button>
                    </div>
                  )}
                </div>
              </>
            ) : (
              <>
                <button
                  type="button"
                  className="navigation-bar__button navigation-bar__button--ghost"
                  onClick={() => navigate('/login')}
                >
                  Sign in
                </button>
                <button
                  type="button"
                  className="navigation-bar__button"
                  onClick={() => navigate('/register')}
                >
                  Sign up
                </button>
              </>
            )}
          </div>
        </div>

        <div className="home-filters">
          {['all', 'matchup', 'bracket'].map((f) => (
            <button
              key={f}
              type="button"
              className={`home-filter-chip${typeFilter === f ? ' home-filter-chip--active' : ''}`}
              onClick={() => setTypeFilter(f)}
            >
              {f === 'all' ? 'All Types' : f.charAt(0).toUpperCase() + f.slice(1)}
            </button>
          ))}
        </div>

        {loading ? (
          <div className="home-card-grid">
            {Array.from({ length: 9 }).map((_, i) => (
              <div key={i} className="home-card home-card--skeleton">
                <div className="home-card__header">
                  <div className="home-skeleton-circle" />
                  <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '0.3rem' }}>
                    <div className="home-skeleton-line" style={{ width: '40%' }} />
                    <div className="home-skeleton-line" style={{ width: '25%', height: '10px' }} />
                  </div>
                </div>
                <div className="home-card__caption">
                  <div className="home-skeleton-line home-skeleton-line--title" />
                  <div className="home-skeleton-line home-skeleton-line--short" />
                </div>
                <div className="home-card__media home-card__media--skeleton" />
                <div className="home-card__actions">
                  <div className="home-card__action-row">
                    <div className="home-skeleton-line" style={{ width: '20%' }} />
                    <div className="home-skeleton-line" style={{ width: '20%' }} />
                    <div className="home-skeleton-line" style={{ width: '20%' }} />
                  </div>
                </div>
              </div>
            ))}
          </div>
        ) : visibleItems.length === 0 ? (
          <div className="home-empty">
            <p>{typeFilter === 'bracket' ? 'No brackets found.' : 'No matchups found.'}</p>
            <button
              type="button"
              className="home-create-btn"
              onClick={() => {
                if (!userId) { navigate('/login'); return; }
                if (typeFilter === 'bracket') {
                  navigate('/brackets/new');
                } else {
                  navigateToCreate();
                }
              }}
            >
              Create the first one
            </button>
          </div>
        ) : (
          <div className="home-card-grid">
            {visibleItems.map((item) => {
              const isLikedByMe = item._type === 'bracket'
                ? likedBracketIds.has(String(item.id))
                : likedMatchupIds.has(String(item.id));
              return (
                <HomeCard
                  key={`${item._type}-${item.id}`}
                  item={item}
                  type={item._type}
                  initialLiked={isLikedByMe}
                />
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};

export default HomePage;
