// frontend/src/pages/HomePage.js
import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getHomeSummary } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import '../styles/HomePage.css';

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [totalEngagements, setTotalEngagements] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [bracketsLoading, setBracketsLoading] = useState(true);
  const [bracketsError, setBracketsError] = useState(null);

  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const navigateToCreateBracket = () => {
    navigate('/brackets/new');
  };
  

  useEffect(() => {
    const fetchHomeSummary = async () => {
      setIsLoading(true);
      setBracketsLoading(true);

      try {
        const response = await getHomeSummary(userId);
        const data = response.data?.response ?? response.data ?? {};
        const matchupsData = data.popular_matchups ?? [];
        const bracketsData = data.popular_brackets ?? [];

        setMatchups(Array.isArray(matchupsData) ? matchupsData : []);
        setBrackets(Array.isArray(bracketsData) ? bracketsData : []);
        setTotalEngagements(Number(data.total_engagements ?? 0));
        setError(null);
        setBracketsError(null);
      } catch (err) {
        console.error('Failed to fetch home summary:', err);
        setError('We could not load popular matchups. Please try again in a moment.');
        setBracketsError('We could not load popular brackets. Please try again in a moment.');
        setTotalEngagements(0);
      } finally {
        setIsLoading(false);
        setBracketsLoading(false);
      }
    };

    fetchHomeSummary();
  }, [userId]);

  const navigateToCreateMatchup = () => {
    const storedUserId = localStorage.getItem('userId');
    if (!storedUserId) return;
    navigate(`/users/${storedUserId}/create-matchup`);
  };

  // (rest of the file remains unchanged)


  useEffect(() => {
    if (!userId) {
      setTotalEngagements(0);
    }
  }, [userId]);

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
          </div>
          <div className="home-hero-card" aria-hidden="true">
            <div className="home-hero-card-ring" />
            <div className="home-hero-card-inner">
              <span className="home-hero-card-label">Your Total Engagements</span>
              <span className="home-hero-card-count">{totalEngagements}</span>
            </div>
          </div>
        </section>

        <section className="home-matchups-section">
          <header className="home-section-header">
            <div>
              <h2>Popular Matchups</h2>
              <p>See what the community is engaging with the most right now.</p>
            </div>
            <div className="home-section-actions">
              <Button onClick={navigateToCreateMatchup} className="home-secondary-button">
                New Matchup
              </Button>
            </div>
          </header>

          {isLoading && (
            <div className="home-status-card">
              <p>Loading popular matchups...</p>
            </div>
          )}

          {!isLoading && (error || matchups.length === 0) && (
            <div className="home-empty-state">
              <h3>No popular matchups yet.</h3>
              <p>
                {error
                  ? 'Popular matchups are not available right now.'
                  : 'Be the first to create a matchup and start the conversation.'}
              </p>
              <div className="home-empty-actions">
                <Button
                  onClick={navigateToCreateMatchup}
                  className="home-create-button home-create-button--ghost"
                >
                  Create a Matchup
                </Button>
              </div>
            </div>
          )}

          {!isLoading && !error && matchups.length > 0 && (
            <div className="home-matchups-grid">
              {matchups.map((matchup) => {
                const matchupId = matchup.id ?? matchup.matchup_id ?? matchup.matchupId;
                const matchupTitle = matchup.title || `Matchup #${matchupId}`;
                const matchupOwnerId =
                  matchup.author_id ?? matchup.user_id ?? matchup.owner_id ?? userId;

                return (
                  <article key={matchupId} className="home-matchup-card">
                    <div className="home-matchup-card-body">
                      <h3>{matchupTitle}</h3>
                      <p className="home-matchup-meta">
                        {typeof matchup.rank !== 'undefined' && `Rank #${matchup.rank}`}
                        {typeof matchup.engagement_score !== 'undefined' &&
                          ` · ${Math.round(matchup.engagement_score)} total engagements`}
                      </p>
                    </div>
                    {matchupOwnerId ? (
                      <Link
                        to={`/users/${matchupOwnerId}/matchup/${matchupId}`}
                        className="home-matchup-link"
                      >
                        View matchup
                        <span className="home-matchup-link-arrow" aria-hidden="true">
                          &gt;
                        </span>
                      </Link>
                    ) : (
                      <span className="home-matchup-link" aria-disabled="true">
                        Owner missing
                      </span>
                    )}
                  </article>
                );
              })}
            </div>
          )}
        </section>

        <section className="home-matchups-section">
          <header className="home-section-header">
            <div>
              <h2>Popular Brackets</h2>
              <p>See which brackets are getting the most attention this round.</p>
            </div>
            <div className="home-section-actions">
              <Button
                onClick={navigateToCreateBracket}
                className="home-secondary-button home-secondary-button--alt"
              >
                New Bracket
              </Button>
            </div>
          </header>

          {bracketsLoading && (
            <div className="home-status-card">
              <p>Loading popular brackets...</p>
            </div>
          )}

          {!bracketsLoading && (bracketsError || brackets.length === 0) && (
            <div className="home-empty-state">
              <h3>No popular brackets yet.</h3>
              <p>
                {bracketsError
                  ? 'Popular brackets are not available right now.'
                  : 'Start a bracket and invite others to vote.'}
              </p>
              <div className="home-empty-actions">
                <Button
                  onClick={navigateToCreateBracket}
                  className="home-create-button home-create-button--ghost"
                >
                  Create a Bracket
                </Button>
              </div>
            </div>
          )}

          {!bracketsLoading && !bracketsError && brackets.length > 0 && (
            <div className="home-matchups-grid">
              {brackets.map((bracket) => {
                const bracketId = bracket.id ?? bracket.bracket_id ?? bracket.bracketId;
                const bracketTitle = bracket.title || `Bracket #${bracketId}`;

                return (
                  <article key={bracketId} className="home-matchup-card">
                    <div className="home-matchup-card-body">
                      <h3>{bracketTitle}</h3>
                      <p className="home-matchup-meta">
                        {typeof bracket.rank !== 'undefined' && `Rank #${bracket.rank}`}
                        {typeof bracket.current_round !== 'undefined' &&
                          ` · Round ${bracket.current_round}`}
                        {typeof bracket.engagement_score !== 'undefined' &&
                          ` · ${Math.round(bracket.engagement_score)} round engagements`}
                      </p>
                    </div>
                    {bracketId ? (
                      <Link to={`/brackets/${bracketId}`} className="home-matchup-link">
                        View bracket
                        <span className="home-matchup-link-arrow" aria-hidden="true">
                          &gt;
                        </span>
                      </Link>
                    ) : (
                      <span className="home-matchup-link" aria-disabled="true">
                        Bracket missing
                      </span>
                    )}
                  </article>
                );
              })}
            </div>
          )}
        </section>
      </main>
    </div>
  );
};

export default HomePage;
