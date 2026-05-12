import React, { useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import NotificationBell from './NotificationBell';
import ThemeToggleItem from './ThemeToggleItem';
import { logout as serverLogout, signOutLocally } from '../services/api';
import './NavigationBar.css';

const NavigationBar = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const isAuthed = Boolean(localStorage.getItem('token'));

  // Avatar dropdown — collapses View profile / Account settings /
  // Logout behind a single click. Removes the previous standalone
  // ⎋ Logout icon from the top nav (logout was a peer to navigation,
  // which adds visual weight; identity/session actions belong tucked
  // into the avatar). Same outside-click + Escape close pattern used
  // elsewhere in the app.
  const [avatarMenuOpen, setAvatarMenuOpen] = useState(false);
  const avatarMenuRef = useRef(null);
  useEffect(() => {
    if (!avatarMenuOpen) return undefined;
    const onPointerDown = (e) => {
      if (avatarMenuRef.current && !avatarMenuRef.current.contains(e.target)) {
        setAvatarMenuOpen(false);
      }
    };
    const onKey = (e) => { if (e.key === 'Escape') setAvatarMenuOpen(false); };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [avatarMenuOpen]);

  const handleLogout = async () => {
    // Best-effort server-side revoke — we don't block the UX on it.
    // Even if the server is down / returns an error, we still clear
    // local auth so the user ends up on /login either way.
    const refreshToken = localStorage.getItem('refresh_token');
    if (refreshToken) {
      try { await serverLogout(refreshToken); } catch { /* ignore */ }
    }
    signOutLocally();
    navigate('/login', { replace: true });
  };

  const profileSlug = (username && username !== 'undefined') ? username : userId;
  // Hide "View profile" when the current route already shows the
  // viewer's own profile. The route is /users/:slug where slug is
  // username-or-uuid, so match either.
  const onOwnProfile = Boolean(profileSlug) && (
    location.pathname === `/users/${profileSlug}` ||
    (userId && location.pathname === `/users/${userId}`)
  );
  const goAndClose = (path) => () => {
    setAvatarMenuOpen(false);
    navigate(path);
  };
  const handleLogoutFromMenu = async () => {
    setAvatarMenuOpen(false);
    await handleLogout();
  };

  return (
    <header className="navigation-bar">
      <div className="navigation-bar__inner">
        <button
          type="button"
          className="navigation-bar__brand"
          onClick={() => navigate('/home')}
        >
          Matchup
        </button>
        <div className="navigation-bar__actions">
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
              {userId && (
                <div className="navigation-bar__profile-menu" ref={avatarMenuRef}>
                  <button
                    type="button"
                    className="navigation-bar__profile-trigger"
                    aria-haspopup="menu"
                    aria-expanded={avatarMenuOpen}
                    aria-label="Open account menu"
                    onClick={() => setAvatarMenuOpen((v) => !v)}
                  >
                    <ProfilePic userId={userId} size={44} />
                  </button>
                  {avatarMenuOpen && (
                    <div className="navigation-bar__profile-panel" role="menu">
                      {/* Mobile-only Home + Admin entries. The standalone
                          Home / Admin buttons in the top row are hidden
                          on small screens (CSS) so the action cluster
                          stays at [bell] [avatar] only — these dropdown
                          items keep both routes reachable. Desktop sees
                          the same items here, which is mildly redundant
                          but harmless. */}
                      <button
                        type="button"
                        className="navigation-bar__profile-item navigation-bar__profile-item--mobile"
                        role="menuitem"
                        onClick={goAndClose('/home')}
                      >
                        Home
                      </button>
                      {localStorage.getItem('isAdmin') === 'true' && (
                        <button
                          type="button"
                          className="navigation-bar__profile-item navigation-bar__profile-item--mobile"
                          role="menuitem"
                          onClick={goAndClose('/admin')}
                        >
                          Admin
                        </button>
                      )}
                      {profileSlug && !onOwnProfile && (
                        <button
                          type="button"
                          className="navigation-bar__profile-item"
                          role="menuitem"
                          onClick={goAndClose(`/users/${profileSlug}`)}
                        >
                          View profile
                        </button>
                      )}
                      <button
                        type="button"
                        className="navigation-bar__profile-item"
                        role="menuitem"
                        onClick={goAndClose('/settings/account')}
                      >
                        Account settings
                      </button>
                      {/* Theme toggle — flips light/dark and persists
                          to localStorage. Sitting inside the avatar
                          menu mirrors the convention from GitHub /
                          Linear / Vercel: surface-level appearance
                          controls live with identity, not as a peer
                          to navigation. */}
                      <ThemeToggleItem
                        className="navigation-bar__profile-item"
                      />
                      <div className="navigation-bar__profile-divider" />
                      <button
                        type="button"
                        className="navigation-bar__profile-item navigation-bar__profile-item--danger"
                        role="menuitem"
                        onClick={handleLogoutFromMenu}
                      >
                        Log out
                      </button>
                    </div>
                  )}
                </div>
              )}
            </>
          ) : (
            <>
              {/* Sign up is the primary anon CTA — anon users browse +
                  vote up to 3 times, and the conversion target is
                  signup. Sign in stays available as a quieter ghost
                  button for returning users. */}
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
    </header>
  );
};

export default NavigationBar;
