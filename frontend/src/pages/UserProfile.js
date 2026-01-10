import React, { useState, useEffect } from 'react';
import { useParams, Link, useNavigate } from 'react-router-dom';
import {
  getUserMatchups,
  getUserBrackets,
  getUser,
  archiveBracket,
} from '../services/api';

import NavigationBar from '../components/NavigationBar';
import ProfilePic from '../components/ProfilePic';
import Button from '../components/Button';
import '../styles/UserProfile.css';

const normalizeToArray = (raw) => {
  if (Array.isArray(raw)) return raw;

  if (raw && typeof raw === 'object') {
    if (Array.isArray(raw.response)) return raw.response;
    if (Array.isArray(raw.data)) return raw.data;
    if (Array.isArray(raw.matchups)) return raw.matchups;
    if (Array.isArray(raw.brackets)) return raw.brackets;

    if (raw.response && typeof raw.response === 'object') {
      if (Array.isArray(raw.response.matchups)) return raw.response.matchups;
      if (Array.isArray(raw.response.brackets)) return raw.response.brackets;
      if (Array.isArray(raw.response.data)) return raw.response.data;
    }
  }

  return [];
};

const extractBracketId = (matchup = {}) =>
  matchup.bracket_id ??
  matchup.bracketId ??
  matchup.BracketID ??
  matchup.bracketID ??
  null;

const isStandaloneMatchup = (matchup = {}) => extractBracketId(matchup) === null;

const UserProfile = () => {
  const { userId } = useParams();
  const navigate = useNavigate();

  const [user, setUser] = useState(null);
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [matchupsError, setMatchupsError] = useState(null);
  const [bracketsError, setBracketsError] = useState(null);

  useEffect(() => {
    let isMounted = true;

    const loadProfile = async () => {
      setLoading(true);
      setError(null);
      setMatchupsError(null);
      setBracketsError(null);

      // 1️⃣ Load USER (this is the ONLY blocking request)
      try {
        const userRes = await getUser(userId);
        if (!isMounted) return;
        setUser(userRes.data.response || userRes.data);
      } catch (err) {
        console.error('Failed to load user', err);
        if (isMounted) {
          setError('Failed to load user data.');
          setLoading(false);
        }
        return;
      }

      // 2️⃣ Load MATCHUPS (non-blocking)
      try {
        const matchupsRes = await getUserMatchups(userId);
        const raw = matchupsRes.data.response ?? matchupsRes.data ?? [];
        const normalized = normalizeToArray(raw);
        setMatchups(normalized.filter(isStandaloneMatchup));
      } catch (err) {
        console.warn('Matchups unavailable', err);
        setMatchups([]);
        setMatchupsError('Matchups unavailable.');
      }

      // 3️⃣ Load BRACKETS (non-blocking)
      try {
        const bracketsRes = await getUserBrackets(userId);
        const raw = bracketsRes.data.response ?? bracketsRes.data ?? [];
        setBrackets(
          normalizeToArray(raw).filter(b => b.status !== "archived")
        );        
      } catch (err) {
        console.warn('Brackets unavailable', err);
        setBrackets([]);
        setBracketsError('Brackets unavailable.');
      }

      if (isMounted) setLoading(false);
    };

    loadProfile();

    return () => {
      isMounted = false;
    };
  }, [userId]);

  const handleDeleteBracket = async (bracket) => {
    if (bracket.status === "active") {
      alert("Active brackets cannot be deleted.");
      return;
    }
  
    const confirmed = window.confirm(
      "Are you sure you want to archive this bracket?"
    );
  
    if (!confirmed) return;
  
    try {
      await archiveBracket(bracket.id);
  
      setBrackets(prev =>
        prev.filter(b => b.id !== bracket.id)
      );
    } catch (err) {
      console.error("Failed to archive bracket", err);
      alert("Failed to archive bracket.");
    }
  };
  
  

  const viewerId = localStorage.getItem('userId');
  const isViewer = viewerId && parseInt(viewerId, 10) === parseInt(userId, 10);

  const matchupEmptyHeading = matchupsError ? 'Matchups unavailable' : 'No matchups yet';
  const matchupEmptyMessage = matchupsError
    ? matchupsError
    : isViewer
      ? 'You have not created a matchup yet. Share your first debate!'
      : 'No matchups to display for this user yet.';

  const bracketEmptyHeading = bracketsError ? 'Brackets unavailable' : 'No brackets yet';
  const bracketEmptyMessage = bracketsError
    ? bracketsError
    : isViewer
      ? 'You have not created a bracket yet. Build your first tournament!'
      : 'No brackets to display for this user yet.';

  const standaloneMatchups = matchups.filter(isStandaloneMatchup);

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
            {/* HERO */}
            <section className="profile-hero">
              <div className="profile-hero-left">
                <ProfilePic userId={userId} editable={isViewer} size={96} />
                <div className="profile-hero-text">
                  <h1>{user.username || user.name || 'Matchup Fan'}</h1>
                  <p className="profile-hero-email">{user.email}</p>
                  <div className="profile-hero-actions">
                    <Button onClick={() => navigate('/home')} className="profile-secondary-button">
                      Back to dashboard
                    </Button>
                    {isViewer && (
                      <>
                        <Button
                          onClick={() => navigate(`/users/${userId}/create-matchup`)}
                          className="profile-primary-button"
                        >
                          Create a matchup
                        </Button>
                        <Button
                          onClick={() => navigate('/brackets/new')}
                          className="profile-secondary-button"
                        >
                          Create a bracket
                        </Button>
                      </>
                    )}
                  </div>
                </div>
              </div>

              <div className="profile-hero-right">
                <div className="profile-stat-card">
                  <span className="profile-stat-label">Matchups</span>
                  <span className="profile-stat-value">{matchups.length}</span>
                </div>
                <div className="profile-stat-card">
                  <span className="profile-stat-label">Brackets</span>
                  <span className="profile-stat-value">{brackets.length}</span>
                </div>
              </div>
            </section>

            {/* MATCHUPS */}
            <section className="profile-section">
              <header className="profile-section-header">
                <div>
                  <h2>{isViewer ? 'Your Recent Matchups' : `${user.username}'s Matchups`}</h2>
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

              <div className="profile-matchups-grid">
                {standaloneMatchups.length > 0 ? (
                  standaloneMatchups.map(m => (
                    <article key={m.id} className="profile-matchup-card">
                      <div className="profile-matchup-card-body">
                        <h3>{m.title}</h3>
                        <p>{(m.content || '').slice(0, 140) || 'No description yet.'}</p>
                      </div>
                      <Link to={`/users/${userId}/matchup/${m.id}`} className="profile-matchup-link">
                        View matchup →
                      </Link>
                    </article>
                  ))
                ) : (
                  <article className="profile-matchup-card profile-matchup-card--empty">
                    <div className="profile-matchup-card-body">
                      <h3>{matchupEmptyHeading}</h3>
                      <p>{matchupEmptyMessage}</p>
                    </div>
                  </article>
                )}
              </div>
            </section>

            {/* BRACKETS */}
            <section className="profile-section">
              <header className="profile-section-header">
                <div>
                  <h2>{isViewer ? 'Your Brackets' : `${user.username}'s Brackets`}</h2>
                  <p>Track ongoing tournaments and bracket history.</p>
                </div>
                {isViewer && (
                  <Button
                    onClick={() => navigate('/brackets/new')}
                    className="profile-secondary-button"
                  >
                    New bracket
                  </Button>
                )}
              </header>

              <div className="profile-matchups-grid">
                {brackets.length > 0 ? (
                  brackets.map(b => (
                    <article key={b.id} className="profile-matchup-card">
                      <div className="profile-matchup-card-body">
                        <h3>{b.title}</h3>
                        <p>{b.description || 'No description yet.'}</p>
                        <p className="profile-matchup-meta">Status: {b.status || 'draft'}</p>
                      </div>

                      <div className="profile-matchup-card-actions">
                        <Link to={`/brackets/${b.id}`} className="profile-matchup-link">
                          View bracket →
                        </Link>
                      </div>

                    </article>

                  ))
                ) : (
                  <article className="profile-matchup-card profile-matchup-card--empty">
                    <div className="profile-matchup-card-body">
                      <h3>{bracketEmptyHeading}</h3>
                      <p>{bracketEmptyMessage}</p>
                    </div>
                  </article>
                )}
              </div>
            </section>
          </>
        )}
      </main>
    </div>
  );
};

export default UserProfile;
