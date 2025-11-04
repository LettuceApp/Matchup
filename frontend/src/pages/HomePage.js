import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getUserMatchups } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import '../styles/HomePage.css';

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchMatchups = async () => {
      const token = localStorage.getItem('token');
      if (!token) {
        return navigate('/login');
      }
      const userId = localStorage.getItem('userId');
      if (!userId) {
        console.error('User ID not found');
        return;
      }

      try {
        const response = await getUserMatchups(userId);
        const matchupsData = response.data.response || response.data;
        setError(null);
        setMatchups(matchupsData);
      } catch (err) {
        console.error('Failed to fetch user matchups:', err);
        setError('We could not load your matchups. Please try again in a moment.');
      } finally {
        setIsLoading(false);
      }
    };

    fetchMatchups();
  }, [navigate]);

  const navigateToCreateMatchup = () => {
    const userId = localStorage.getItem('userId');
    navigate(`/users/${userId}/create-matchup`);
  };

  const userId = localStorage.getItem('userId');

  return (
    <div className="home-page">
      <NavigationBar />

      <main className="home-content">
        <section className="home-hero">
          <div className="home-hero-text">
            <p className="home-overline">Matchup Hub</p>
            <h1>Discover how your matchups stack up.</h1>
            <p className="home-subtitle">
              Track community sentiment, share your thoughts, and keep every matchup in one vibrant dashboard.
            </p>
            <Button onClick={navigateToCreateMatchup} className="home-create-button">
              Create a Matchup
            </Button>
          </div>
          <div className="home-hero-card" aria-hidden="true">
            <div className="home-hero-card-ring" />
            <div className="home-hero-card-inner">
              <span className="home-hero-card-label">Total Matchups</span>
              <span className="home-hero-card-count">{matchups.length}</span>
            </div>
          </div>
        </section>

        <section className="home-matchups-section">
          <header className="home-section-header">
            <div>
              <h2>Your Matchups</h2>
              <p>Jump back into the conversations that matter most to you.</p>
            </div>
            <Button onClick={navigateToCreateMatchup} className="home-secondary-button">
              New Matchup
            </Button>
          </header>

          {isLoading && (
            <div className="home-status-card">
              <p>Loading your matchups...</p>
            </div>
          )}

          {error && !isLoading && (
            <div className="home-status-card home-status-error">
              <p>{error}</p>
            </div>
          )}

          {!isLoading && !error && matchups.length === 0 && (
            <div className="home-empty-state">
              <h3>You have not created any matchups yet.</h3>
              <p>Start a new matchup to spark the conversation.</p>
              <Button onClick={navigateToCreateMatchup} className="home-create-button home-create-button--ghost">
                Create your first matchup
              </Button>
            </div>
          )}

          {!isLoading && !error && matchups.length > 0 && (
            <div className="home-matchups-grid">
              {matchups.map((matchup) => (
                <article key={matchup.id} className="home-matchup-card">
                  <div className="home-matchup-card-body">
                    <h3>{matchup.title}</h3>
                  </div>
                  <Link to={`/users/${userId}/matchup/${matchup.id}`} className="home-matchup-link">
                    View matchup
                    <span className="home-matchup-link-arrow" aria-hidden="true">
                      &gt;
                    </span>
                  </Link>
                </article>
              ))}
            </div>
          )}
        </section>
      </main>
    </div>
  );
};

export default HomePage;
