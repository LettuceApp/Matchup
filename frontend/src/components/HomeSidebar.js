import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import { logout as serverLogout, signOutLocally } from '../services/api';
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
  const allCategoriesEntry = CATEGORIES[0];
  const bodyCategories = CATEGORIES.slice(1);
  const topBodyCategories = bodyCategories.slice(0, COLLAPSED_CATEGORY_COUNT);
  const hiddenBodyCategories = bodyCategories.slice(COLLAPSED_CATEGORY_COUNT);
  const activeIsHidden = hiddenBodyCategories.includes(categoryFilter);
  const showHidden = categoriesExpanded || activeIsHidden;

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

        {/* Communities section — only the entry point in v1. Browse
            directory + "my communities" land in a later phase, by
            which point this will grow into a multi-item section. */}
        {isAuthed && (
          <>
            <div className="home-sidebar__section-label">Communities</div>
            <button
              type="button"
              className="home-sidebar__nav-item"
              onClick={() => navigate('/communities/new')}
            >
              + Create community
            </button>
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
