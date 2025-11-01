import React from 'react';
import { useNavigate, Link } from 'react-router-dom';
import ProfilePic from './ProfilePic';
import './NavigationBar.css';

const NavigationBar = () => {
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    navigate('/login');
  };

  return (
    <header className="navigation-bar">
      <div className="navigation-bar__inner">
        <button
          type="button"
          className="navigation-bar__brand"
          onClick={() => navigate('/')}
        >
          Matchup Hub
        </button>
        <div className="navigation-bar__actions">
          <button
            type="button"
            className="navigation-bar__button"
            onClick={() => navigate('/')}
          >
            Home
          </button>
          <button
            type="button"
            className="navigation-bar__button navigation-bar__button--ghost"
            onClick={handleLogout}
          >
            Logout
          </button>
          {userId && (
            <Link
              to={`/users/${userId}/profile`}
              className="navigation-bar__profile"
              aria-label="View profile"
            >
              <ProfilePic userId={userId} size={44} />
            </Link>
          )}
        </div>
      </div>
    </header>
  );
};

export default NavigationBar;
