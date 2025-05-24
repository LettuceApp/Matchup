import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getUserMatchups } from '../services/api';
import NavigationBar from '../components/NavigationBar';
import Button from '../components/Button';
import ProfilePic from '../components/ProfilePic';      // ← import your component

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
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
        setMatchups(matchupsData);
      } catch (err) {
        console.error('Failed to fetch user matchups:', err);
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
    <div style={{ position: 'relative', paddingTop: '1rem' }}>
      <NavigationBar />

      {/* ← ProfilePic in top-right, wrapped in a Link to the profile */}
      {userId && (
        <Link
          to={`/users/${userId}/profile`}
          style={{
            position: 'fixed',
            top: '1rem',
            right: '1rem',
            width: 40,
            height: 40,
            borderRadius: '50%',
            overflow: 'hidden',
            display: 'block',
            cursor: 'pointer'
          }}
        >
          <ProfilePic userId={userId} size={40} />
        </Link>
      )}

      <h1>Your Matchups</h1>
      {matchups.length > 0 ? (
        matchups.map((matchup) => (
          <div key={matchup.id}>
            <h2>
              <Link to={`/users/${userId}/matchup/${matchup.id}`}>
                {matchup.title}
              </Link>
            </h2>
            <p>{matchup.description}</p>
          </div>
        ))
      ) : (
        <p>No matchups available.</p>
      )}
      
      <Button onClick={navigateToCreateMatchup}>Create a Matchup</Button>
    </div>
  );
};

export default HomePage;
