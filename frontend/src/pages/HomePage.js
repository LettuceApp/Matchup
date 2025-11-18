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

  // Pagination state
  const [page, setPage] = useState(1);
  const [pagination, setPagination] = useState(null);

  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');

  useEffect(() => {
    const fetchMatchups = async () => {
      setIsLoading(true);

      const storedUserId = localStorage.getItem('userId');
      if (!storedUserId) {
        console.error('User ID not found');
        setError('We could not determine your account. Please log in again.');
        setIsLoading(false);
        return;
      }

      try {
        // Use the paginated API: getUserMatchups(userId, page, limit)
        const response = await getUserMatchups(storedUserId, page, 10);
        const data = response.data;

        // Backend may respond with { status, response, pagination } or just the array
        const matchupsData = data.response || data;

        setMatchups(matchupsData);
        setPagination(data.pagination || null);
        setError(null);
      } catch (err) {
        console.error('Failed to fetch user matchups:', err);
        setError('We could not load your matchups. Please try again in a moment.');
      } finally {
        setIsLoading(false);
      }
    };

    fetchMatchups();
  }, [page]);

  const navigateToCreateMatchup = () => {
    const storedUserId = localStorage.getItem('userId');
    if (!storedUserId) {
      return;
    }
    // This is your frontend route (React Router), not the API route
    navigate(`/users/${storedUserId}/create-matchup`);
  };

  const totalMatchups =
    pagination && typeof pagination.total === 'number'
      ? pagination.total
      : matchups.length;

  return (
    <div className="home-page">
      <NavigationBar />

      <main className="home-content">
        <section className="home-hero">
          <div className="home-hero-text">
            <p className="home-overline">Matchup Hub</p>
            <h1>Discover how your matchups stack up.</h1>
            <p className="home-subtitle">
              Track community sentiment, share your thoughts, and keep every matchup in one vibrant
              dashboard.
            </p>
            <Button onClick={navigateToCreateMatchup} className="home-create-button">
              Create a Matchup
            </Button>
          </div>
          <div className="home-hero-card" aria-hidden="true">
            <div className="home-hero-card-ring" />
            <div className="home-hero-card-inner">
              <span className="home-hero-card-label">Total Matchups</span>
              <span className="home-hero-card-count">{totalMatchups}</span>
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
              <Button
                onClick={navigateToCreateMatchup}
                className="home-create-button home-create-button--ghost"
              >
                Create your first matchup
              </Button>
            </div>
          )}

          {!isLoading && !error && matchups.length > 0 && (
            <>
              <div className="home-matchups-grid">
                {matchups.map((matchup) => (
                  <article key={matchup.id} className="home-matchup-card">
                    <div className="home-matchup-card-body">
                      <h3>{matchup.title}</h3>
                    </div>
                    <Link
                      to={`/users/${userId}/matchup/${matchup.id}`}
                      className="home-matchup-link"
                    >
                      View matchup
                      <span className="home-matchup-link-arrow" aria-hidden="true">
                        &gt;
                      </span>
                    </Link>
                  </article>
                ))}
              </div>

              {pagination && pagination.total_pages > 1 && (
                <div className="home-pagination">
                  <button
                    disabled={page <= 1}
                    onClick={() => setPage((prev) => Math.max(1, prev - 1))}
                  >
                    Previous
                  </button>

                  <span>
                    Page {pagination.page} of {pagination.total_pages}
                  </span>

                  <button
                    disabled={pagination.page >= pagination.total_pages}
                    onClick={() => setPage((prev) => prev + 1)}
                  >
                    Next
                  </button>
                </div>
              )}
            </>
          )}
        </section>
      </main>
    </div>
  );
};

export default HomePage;
