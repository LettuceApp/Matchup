import React, { useState, useEffect, useMemo } from 'react';
import { useNavigate } from 'react-router-dom';
import { getHomeSummary, getMatchups, getPopularMatchups } from '../services/api';
import HomeSidebar from '../components/HomeSidebar';
import HomeCard, { deriveTags } from '../components/HomeCard';
import '../styles/HomePage.css';

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
          setMatchups(Array.isArray(summaryData.popular_matchups) ? summaryData.popular_matchups : []);
          setBrackets(homeBrackets);
        } else {
          // most-played: popular matchups sorted by votes desc
          const res = await getPopularMatchups();
          const payload = res.data?.matchups ?? res.data?.response ?? res.data ?? [];
          const sorted = (Array.isArray(payload) ? payload : []).slice().sort(
            (a, b) => (b.votes ?? 0) - (a.votes ?? 0)
          );
          setMatchups(sorted);
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
        const tags = deriveTags(item.title ?? '');
        return tags.includes(categoryFilter);
      });
    }
    return items;
  }, [allItems, searchQuery, categoryFilter]);

  const navigateToCreate = () => {
    if (!userId) { navigate('/login'); return; }
    navigate(`/users/${username || userId}/create-matchup`);
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
            + Create
          </button>
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
                <div className="home-card__thumb home-card__thumb--skeleton" />
                <div className="home-card__body">
                  <div className="home-skeleton-line home-skeleton-line--title" />
                  <div className="home-skeleton-line" />
                  <div className="home-skeleton-line home-skeleton-line--short" />
                </div>
              </div>
            ))}
          </div>
        ) : visibleItems.length === 0 ? (
          <div className="home-empty">
            <p>No matchups found.</p>
            <button type="button" className="home-create-btn" onClick={navigateToCreate}>
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
