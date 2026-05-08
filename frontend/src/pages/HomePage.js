import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getHomeSummary, getMatchups, logout as serverLogout, signOutLocally } from '../services/api';
import HomeSidebar from '../components/HomeSidebar';
import HomeCard, { deriveTags } from '../components/HomeCard';
import NotificationBell from '../components/NotificationBell';
import ProfilePic from '../components/ProfilePic';
import { track } from '../utils/analytics';
import '../styles/HomePage.css';
import '../components/NavigationBar.css';

const HomePage = () => {
  const [sortMode, setSortMode] = useState('latest');
  const [typeFilter, setTypeFilter] = useState('all');
  const [categoryFilter, setCategoryFilter] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [loading, setLoading] = useState(true);

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

  return (
    <div className="home-page">
      <HomeSidebar
        sortMode={sortMode}
        onSortChange={setSortMode}
        categoryFilter={categoryFilter}
        onCategoryChange={setCategoryFilter}
      />

      <div className="home-main">
        <div className="home-topbar">
          <input
            type="text"
            className="home-search"
            placeholder="Search matchups..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
          />
          <button
            type="button"
            className="home-create-btn"
            onClick={navigateToCreate}
          >
            + Create Matchup
          </button>
          <button
            type="button"
            className="home-create-btn home-create-btn--secondary"
            onClick={navigateToCreateBracket}
          >
            + Create Bracket
          </button>

          {/* Nav actions inlined into the topbar — mirrors what
              <NavigationBar /> renders on every other page (Home,
              bell, optional Admin, logout, profile pic). The home
              page doesn't mount NavigationBar at the top because
              the sidebar handles the home/sort/category nav, but
              the user still wants quick access to the same shortcuts
              they have everywhere else. Reuses NavigationBar's
              existing class names + styles so the look is identical. */}
          <div className="home-topbar__actions">
            {isAuthed ? (
              <>
                <button
                  type="button"
                  className="navigation-bar__button"
                  onClick={() => navigate('/home')}
                >
                  Home
                </button>
                <NotificationBell />
                {localStorage.getItem('isAdmin') === 'true' && (
                  <button
                    type="button"
                    className="navigation-bar__button navigation-bar__button--muted"
                    onClick={() => navigate('/admin')}
                  >
                    Admin
                  </button>
                )}
                <button
                  type="button"
                  className="navigation-bar__button navigation-bar__button--icon"
                  onClick={handleLogout}
                  title="Logout"
                >
                  ⎋
                </button>
                {userId && (
                  <Link
                    to={`/users/${(username && username !== 'undefined') ? username : userId}`}
                    className="navigation-bar__profile"
                    aria-label="View profile"
                  >
                    <ProfilePic userId={userId} size={44} />
                  </Link>
                )}
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
            {visibleItems.map((item) => (
              <HomeCard
                key={`${item._type}-${item.id}`}
                item={item}
                type={item._type}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default HomePage;
