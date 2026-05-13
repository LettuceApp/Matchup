import React, { useCallback, useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { search as searchApi } from '../services/api';
import ShareButton from '../components/ShareButton';
import '../styles/SearchResultsPage.css';

/*
 * SearchResultsPage — `/search?q=<query>&f=<tab>`
 *
 * Twitter-style universal search:
 *   - Tabs across the top: Top / Matchups / Brackets / Communities / People
 *   - "Top" fans out to all four entity types and shows the best of each
 *   - Each entity tab calls Search with `only=<type>` for a bigger slice
 *   - Result cards each carry a Share button (copy link)
 *
 * URL params:
 *   q  = query string (required; empty page if missing/short)
 *   f  = filter / active tab. One of 'top' | 'matchups' | 'brackets' |
 *        'communities' | 'people'. Defaults to 'top'.
 */

const TABS = [
  { key: 'top', label: 'Top' },
  { key: 'matchups', label: 'Matchups' },
  { key: 'brackets', label: 'Brackets' },
  { key: 'communities', label: 'Communities' },
  { key: 'people', label: 'People' },
];

const tabToOnly = {
  top: '',
  matchups: 'matchup',
  brackets: 'bracket',
  communities: 'community',
  people: 'user',
};

const SearchResultsPage = () => {
  const [searchParams, setSearchParams] = useSearchParams();
  const query = (searchParams.get('q') || '').trim();
  const activeTab = searchParams.get('f') || 'top';

  const [data, setData] = useState({ matchups: [], brackets: [], communities: [], users: [] });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  // Local input state for the in-page search box. Lets the user
  // re-search from the results page without bouncing back to the
  // home topbar.
  const [inputValue, setInputValue] = useState(query);
  useEffect(() => setInputValue(query), [query]);

  const runSearch = useCallback(async () => {
    if (!query || query.length < 2) {
      setData({ matchups: [], brackets: [], communities: [], users: [] });
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const only = tabToOnly[activeTab] ?? '';
      // Top tab asks for fewer of each type since we're mixing; tab
      // views get the full per-type limit.
      const limit = activeTab === 'top' ? 5 : 25;
      const res = await searchApi({ query, only, limit });
      const payload = res?.data ?? {};
      setData({
        matchups: payload.matchups || [],
        brackets: payload.brackets || [],
        communities: payload.communities || [],
        users: payload.users || [],
      });
    } catch (err) {
      console.warn('Search failed', err);
      setError('Could not load search results.');
    } finally {
      setLoading(false);
    }
  }, [query, activeTab]);

  useEffect(() => {
    runSearch();
  }, [runSearch]);

  const handleSubmit = (e) => {
    e.preventDefault();
    const next = inputValue.trim();
    if (!next) return;
    setSearchParams({ q: next, f: activeTab });
  };

  const switchTab = (tab) => {
    setSearchParams({ q: query, f: tab });
  };

  const noResults =
    !loading &&
    !error &&
    query.length >= 2 &&
    data.matchups.length === 0 &&
    data.brackets.length === 0 &&
    data.communities.length === 0 &&
    data.users.length === 0;

  return (
    <div className="search-results-page">
      <form className="search-results__form" onSubmit={handleSubmit}>
        <span className="search-results__icon" aria-hidden="true">🔍</span>
        <input
          type="search"
          className="search-results__input"
          placeholder="Search Matchup…"
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          autoFocus
        />
      </form>

      <nav className="search-results__tabs" aria-label="Search filters">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            className={`search-results__tab ${t.key === activeTab ? 'is-active' : ''}`}
            onClick={() => switchTab(t.key)}
          >
            {t.label}
          </button>
        ))}
      </nav>

      {query.length < 2 && (
        <div className="search-results__empty search-results__empty--landing">
          <h2 className="search-results__landing-title">Search Matchup</h2>
          <p>One search across matchups, brackets, communities, and people.</p>
          <p className="search-results__hint">
            Start typing above — we'll fan out across every public surface
            and show the best of each in <strong>Top</strong>, with deeper
            slices in the other tabs.
          </p>
        </div>
      )}

      {loading && query.length >= 2 && (
        <div className="search-results__empty">Searching…</div>
      )}

      {error && <div className="search-results__error">{error}</div>}

      {noResults && (
        <div className="search-results__empty">
          <p>No results for <strong>“{query}”</strong>.</p>
          <p className="search-results__hint">Try a different word or check spelling.</p>
        </div>
      )}

      {!loading && !error && query.length >= 2 && (
        <div className="search-results__content">
          {(activeTab === 'top' || activeTab === 'matchups') && data.matchups.length > 0 && (
            <Section
              title="Matchups"
              showAll={activeTab === 'top' && data.matchups.length === 5}
              onShowAll={() => switchTab('matchups')}
            >
              {data.matchups.map((m) => (
                <MatchupResult key={m.id} item={m} />
              ))}
            </Section>
          )}

          {(activeTab === 'top' || activeTab === 'brackets') && data.brackets.length > 0 && (
            <Section
              title="Brackets"
              showAll={activeTab === 'top' && data.brackets.length === 5}
              onShowAll={() => switchTab('brackets')}
            >
              {data.brackets.map((b) => (
                <BracketResult key={b.id} item={b} />
              ))}
            </Section>
          )}

          {(activeTab === 'top' || activeTab === 'communities') && data.communities.length > 0 && (
            <Section
              title="Communities"
              showAll={activeTab === 'top' && data.communities.length === 5}
              onShowAll={() => switchTab('communities')}
            >
              {data.communities.map((c) => (
                <CommunityResult key={c.id} item={c} />
              ))}
            </Section>
          )}

          {(activeTab === 'top' || activeTab === 'people') && data.users.length > 0 && (
            <Section
              title="People"
              showAll={activeTab === 'top' && data.users.length === 5}
              onShowAll={() => switchTab('people')}
            >
              {data.users.map((u) => (
                <UserResult key={u.id} item={u} />
              ))}
            </Section>
          )}
        </div>
      )}
    </div>
  );
};

const Section = ({ title, showAll, onShowAll, children }) => (
  <section className="search-results__section">
    <header className="search-results__section-header">
      <h2>{title}</h2>
      {showAll && (
        <button type="button" className="search-results__see-all" onClick={onShowAll}>
          See all →
        </button>
      )}
    </header>
    <div className="search-results__list">{children}</div>
  </section>
);

const MatchupResult = ({ item }) => (
  <article className="search-result-card">
    <Link to={`/users/${item.author_username}/matchup/${item.id}`} className="search-result-card__main">
      {item.image_url ? (
        <img src={item.image_url} alt="" className="search-result-card__thumb" />
      ) : (
        <div className="search-result-card__thumb search-result-card__thumb--gradient" aria-hidden="true" />
      )}
      <div className="search-result-card__text">
        <h3 className="search-result-card__title">{item.title}</h3>
        <p className="search-result-card__meta">
          by <strong>@{item.author_username}</strong> · {item.likes_count} ♥ · {item.comments_count} 💬
        </p>
        {item.tags?.length > 0 && (
          <div className="search-result-card__tags">
            {item.tags.slice(0, 3).map((t) => (
              <span key={t} className="search-result-card__tag">#{t}</span>
            ))}
          </div>
        )}
      </div>
    </Link>
    <div className="search-result-card__actions">
      <ShareButton item={item} type="matchup" />
    </div>
  </article>
);

const BracketResult = ({ item }) => (
  <article className="search-result-card">
    <Link to={`/brackets/${item.id}`} className="search-result-card__main">
      <div className="search-result-card__thumb search-result-card__thumb--gradient" aria-hidden="true">
        <span className="search-result-card__bracket-size">{item.size}</span>
      </div>
      <div className="search-result-card__text">
        <h3 className="search-result-card__title">{item.title}</h3>
        <p className="search-result-card__meta">
          by <strong>@{item.author_username}</strong> · {item.size} contenders · {item.likes_count} ♥
        </p>
        {item.description && (
          <p className="search-result-card__desc">{item.description}</p>
        )}
      </div>
    </Link>
    <div className="search-result-card__actions">
      <ShareButton item={item} type="bracket" />
    </div>
  </article>
);

const CommunityResult = ({ item }) => (
  <Link to={`/c/${item.slug}`} className="search-result-card">
    <div className="search-result-card__main">
      <div className="search-result-card__community-avatar" aria-hidden="true">
        {item.avatar_path ? <img src={item.avatar_path} alt="" /> : <span>{(item.name || '?').charAt(0).toUpperCase()}</span>}
      </div>
      <div className="search-result-card__text">
        <h3 className="search-result-card__title">{item.name}</h3>
        <p className="search-result-card__meta">
          /c/{item.slug} · <strong>{item.member_count}</strong> {item.member_count === 1 ? 'member' : 'members'}
        </p>
        {item.description && (
          <p className="search-result-card__desc">{item.description}</p>
        )}
      </div>
    </div>
  </Link>
);

const UserResult = ({ item }) => (
  <Link to={`/users/${item.username}`} className="search-result-card">
    <div className="search-result-card__main">
      <div className="search-result-card__user-avatar" aria-hidden="true">
        {item.avatar_path ? <img src={item.avatar_path} alt="" /> : <span>{(item.username || '?').charAt(0).toUpperCase()}</span>}
      </div>
      <div className="search-result-card__text">
        <h3 className="search-result-card__title">@{item.username}</h3>
        <p className="search-result-card__meta">
          <strong>{item.followers_count}</strong> {item.followers_count === 1 ? 'follower' : 'followers'}
        </p>
        {item.bio && <p className="search-result-card__desc">{item.bio}</p>}
      </div>
    </div>
  </Link>
);

export default SearchResultsPage;
