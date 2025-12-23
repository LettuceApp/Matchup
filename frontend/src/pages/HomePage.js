// frontend/src/pages/HomePage.js
import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getPopularMatchups, getUserMatchups, getMatchupLikes } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import '../styles/HomePage.css';

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
  const [totalEngagements, setTotalEngagements] = useState(0);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);

  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');

  useEffect(() => {
    const fetchPopularMatchups = async () => {
      setIsLoading(true);

      try {
        const response = await getPopularMatchups();
        const data = response.data;

        // Backend is now responsible for returning exactly what we need (top 3)
        const matchupsData = data.response || data;
        setMatchups(matchupsData || []);
        setError(null);
      } catch (err) {
        console.error('Failed to fetch popular matchups:', err);
        setError('We could not load popular matchups. Please try again in a moment.');
      } finally {
        setIsLoading(false);
      }
    };

    fetchPopularMatchups();
  }, []);

  const navigateToCreateMatchup = () => {
    const storedUserId = localStorage.getItem('userId');
    if (!storedUserId) return;
    navigate(`/users/${storedUserId}/create-matchup`);
  };

  // (rest of the file remains unchanged)


  useEffect(() => {
    const fetchUserEngagements = async () => {
      if (!userId) {
        setTotalEngagements(0);
        return;
      }

      let page = 1;
      const limit = 25;
      let totalPages = 1;
      let engagementAccumulator = 0;

      const currentUserId = Number(userId);
      const calculateEngagement = (matchup, likesArray = []) => {
        const filteredComments = Array.isArray(matchup.comments)
          ? matchup.comments.filter(
              (comment) => Number(comment.user_id) !== currentUserId
            )
          : [];
        const filteredLikes = Array.isArray(likesArray)
          ? likesArray.filter((like) => Number(like.user_id) !== currentUserId)
          : [];
        const votes = Array.isArray(matchup.items)
          ? matchup.items.reduce((sum, item) => sum + (Number(item.votes) || 0), 0)
          : 0;
        return filteredLikes.length + filteredComments.length + votes;
      };

      const fetchLikesForMatchups = async (matchupsList) => {
        const likesMap = {};
        await Promise.all(
          matchupsList.map(async (matchup) => {
            try {
              const res = await getMatchupLikes(matchup.id);
              likesMap[matchup.id] = res.data?.response || [];
            } catch (err) {
              console.error(`Failed to load likes for matchup ${matchup.id}:`, err);
              likesMap[matchup.id] = [];
            }
          })
        );
        return likesMap;
      };

      try {
        do {
          const response = await getUserMatchups(userId, page, limit);
          const payload = response.data || {};
          const matchupsData = payload.response || [];
          const paginationInfo = payload.pagination || {};
          totalPages = paginationInfo.total_pages || 1;
          const likesMap = await fetchLikesForMatchups(matchupsData);

          engagementAccumulator += matchupsData.reduce((sum, matchup) => {
            const likesForMatchup = likesMap[matchup.id] || [];
            return sum + calculateEngagement(matchup, likesForMatchup);
          }, 0);

          page += 1;
        } while (page <= totalPages);

        setTotalEngagements(engagementAccumulator);
      } catch (err) {
        console.error('Failed to calculate user engagements:', err);
        setTotalEngagements(0);
      }
    };

    fetchUserEngagements();
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
            <Button onClick={navigateToCreateMatchup} className="home-create-button">
              Create a Matchup
            </Button>
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
            <Button onClick={navigateToCreateMatchup} className="home-secondary-button">
              New Matchup
            </Button>
          </header>

          {isLoading && (
            <div className="home-status-card">
              <p>Loading popular matchups...</p>
            </div>
          )}

          {error && !isLoading && (
            <div className="home-status-card home-status-error">
              <p>{error}</p>
            </div>
          )}

          {!isLoading && !error && matchups.length === 0 && (
            <div className="home-empty-state">
              <h3>No popular matchups yet.</h3>
              <p>Be the first to create a matchup and start the conversation.</p>
              <Button
                onClick={navigateToCreateMatchup}
                className="home-create-button home-create-button--ghost"
              >
                Create a matchup
              </Button>
            </div>
          )}

          {!isLoading && !error && matchups.length > 0 && (
            <div className="home-matchups-grid">
              {matchups.map((matchup) => {
                const matchupOwnerId =
                  matchup.author_id ?? matchup.user_id ?? matchup.owner_id ?? userId;

                return (
                  <article key={matchup.id} className="home-matchup-card">
                    <div className="home-matchup-card-body">
                      <h3>{matchup.title}</h3>
                      {typeof matchup.rank !== 'undefined' && (
                        <p className="home-matchup-meta">
                          Rank #{matchup.rank}
                          {typeof matchup.engagement_score !== 'undefined' &&
                            ` · ${Math.round(matchup.engagement_score)} total engagements`}
                        </p>
                      )}
                    </div>
                    {matchupOwnerId ? (
                      <Link
                        to={`/users/${matchupOwnerId}/matchup/${matchup.id}`}
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
      </main>
    </div>
  );
};

export default HomePage;
