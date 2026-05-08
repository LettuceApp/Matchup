import React from 'react';
import { useNavigate } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import { logout as serverLogout, signOutLocally } from '../services/api';

// Order mirrors the user's preferred ordering. "All Categories" stays
// pinned to the top as the cleared-filter; "Other" pinned to the
// bottom as the catch-all. Each entry must match a TAG_RULES tag in
// HomeCard.js so deriveTags() produces a string the filter compares
// against (HomePage.js does an exact `tags.includes(categoryFilter)`).
const CATEGORIES = [
  'All Categories',
  'Anime',
  'Manga',
  'Gaming',
  'Music',
  'Movies',
  'TV Shows',
  'K-Pop',
  'Sports',
  'Food',
  'Pokémon',
  'Cartoons',
  'Animals',
  'Celebrities',
  'Other',
];

const HomeSidebar = ({ sortMode, onSortChange, categoryFilter, onCategoryChange }) => {
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const isAuthed = Boolean(localStorage.getItem('token'));

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
    <aside className="home-sidebar">
      <div className="home-sidebar__brand" onClick={() => navigate('/home')}>
        Matchup Hub
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
        {CATEGORIES.map((cat) => {
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
