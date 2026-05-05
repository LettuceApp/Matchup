import React from 'react';
import { useNavigate, Link } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import NotificationBell from './NotificationBell';
import { logout as serverLogout, signOutLocally } from '../services/api';
import './NavigationBar.css';

const NavigationBar = () => {
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const isAuthed = Boolean(localStorage.getItem('token'));

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

  return (
    <header className="navigation-bar">
      <div className="navigation-bar__inner">
        <button
          type="button"
          className="navigation-bar__brand"
          onClick={() => navigate('/home')}
        >
          Matchup Hub
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
