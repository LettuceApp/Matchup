import React from 'react';
import { useNavigate, Link } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import './NavigationBar.css';

const NavigationBar = () => {
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const username = localStorage.getItem('username');
  const isAuthed = Boolean(localStorage.getItem('token'));

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    localStorage.removeItem('username');
    localStorage.removeItem('isAdmin');
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
              {localStorage.getItem('isAdmin') === 'true' && (
                <button
                  type="button"
                  className="navigation-bar__button"
                  onClick={() => navigate('/admin')}
                >
                  Admin
                </button>
              )}
              <button
                type="button"
                className="navigation-bar__button navigation-bar__button--ghost"
                onClick={handleLogout}
              >
                Logout
              </button>
              {userId && (
                <Link
                  to={`/users/${username || userId}`}
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
                className="navigation-bar__button"
                onClick={() => navigate('/login')}
              >
                Sign in
              </button>
              <button
                type="button"
                className="navigation-bar__button navigation-bar__button--ghost"
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
