import React, { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import { getUserMatchups, getUser } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import ProfilePic from '../components/ProfilePic';
import Button from '../components/Button';
import './UserProfile.css';

const UserProfile = () => {
  const { userId } = useParams();
  const navigate = useNavigate();
  const [matchups, setMatchups] = useState([]);
  const [user, setUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    (async () => {
      try {
        const [uRes, mRes] = await Promise.all([
          getUser(userId),
          getUserMatchups(userId)
        ]);
        setUser(uRes.data.response || uRes.data);
        setMatchups(mRes.data.response || mRes.data);
      } catch (err) {
        console.error(err);
        setError('Failed to load user data.');
      } finally {
        setLoading(false);
      }
    })();
  }, [userId]);

  const viewerId = localStorage.getItem('userId');
  const isViewer = viewerId && parseInt(viewerId, 10) === parseInt(userId, 10);

  return (
    <div className="profile-page">
      <NavigationBar />

      <main className="profile-content">
        {loading && (
          <div className="profile-status-card">Loading profile…</div>
        )}

        {error && !loading && (
          <div className="profile-status-card profile-status-card--error">
            {error}
          </div>
        )}

        {!loading && !error && user && (
          <>
            <section className="profile-hero">
              <div className="profile-hero-left">
                <div className="profile-avatar-wrapper">
                  <ProfilePic
                    userId={userId}
                    editable={isViewer}
                    size={96}
                  />
                </div>
                <div className="profile-hero-text">
                  <h1>{user.username || user.name || 'Matchup Fan'}</h1>
                  <p className="profile-hero-email">{user.email}</p>
                  <div className="profile-hero-actions">
                    <Button
                      onClick={() => navigate('/')}
                      className="profile-secondary-button"
                    >
                      Back to dashboard
                    </Button>
                    {isViewer && (
                      <Button
                        onClick={() => navigate(`/users/${userId}/create-matchup`)}
                        className="profile-primary-button"
                      >
                        Create a matchup
                      </Button>
                    )}
                  </div>
                </div>
              </div>
              <div className="profile-hero-right">
                <div className="profile-stat-card">
                  <span className="profile-stat-label">Matchups</span>
                  <span className="profile-stat-value">{matchups.length}</span>
                </div>
              </div>
            </section>

            <section className="profile-section">
              <header className="profile-section-header">
                <div>
                  <h2>{isViewer ? 'Your Recent Matchups' : `${user.username || 'This user'}'s Matchups`}</h2>
                  <p>Explore the latest head-to-heads crafted by this user.</p>
                </div>
                {isViewer && (
                  <Button
                    onClick={() => navigate(`/users/${userId}/create-matchup`)}
                    className="profile-secondary-button"
                  >
                    New matchup
                  </Button>
                )}
              </header>

              {matchups.length > 0 ? (
                <div className="profile-matchups-grid">
                  {matchups.map((m) => (
                    <article key={m.id} className="profile-matchup-card">
                      <div className="profile-matchup-card-body">
                        <h3>{m.title}</h3>
                        <p>{(m.content || m.description || '').slice(0, 140) || 'No description yet.'}</p>
                      </div>
                      <Link
                        to={`/users/${userId}/matchup/${m.id}`}
                        className="profile-matchup-link"
                      >
                        View matchup
                        <span aria-hidden="true">&gt;</span>
                      </Link>
                    </article>
                  ))}
                </div>
              ) : (
                <div className="profile-status-card profile-status-card--muted">
                  {isViewer
                    ? 'You have not created a matchup yet. Share your first debate!'
                    : 'No matchups to display for this user yet.'}
                </div>
              )}
            </section>
          </>
        )}
      </main>
    </div>
  );
};

export default UserProfile;
