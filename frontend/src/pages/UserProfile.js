// UserProfile.js
import React, { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { getUserMatchups } from '../services/api';
import NavigationBar from '../components/NavigationBar';

const UserProfile = () => {
  const { userId } = useParams(); // Extract the userId from the URL
  const [matchups, setMatchups] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchUserMatchups = async () => {
      try {
        const response = await getUserMatchups(userId);
        setMatchups(response.data.response || response.data); // Adjust based on response structure
      } catch (err) {
        console.error('Failed to fetch user matchups:', err);
      } finally {
        setLoading(false);
      }
    };

    fetchUserMatchups();
  }, [userId]);

  if (loading) {
    return <p>Loading matchups...</p>;
  }

  return (
    <div>
      <NavigationBar />
      <h1>User Profile</h1>
      <h2>Matchups by User {userId}</h2>

      {matchups.length > 0 ? (
        matchups.map((matchup) => (
          <div key={matchup.id}>
            <h3>
              <Link to={`/users/${userId}/matchup/${matchup.id}`}>{matchup.title}</Link> {/* Link to matchup page */}
            </h3>
            <p>{matchup.content}</p> {/* Adjust field name if needed */}
          </div>
        ))
      ) : (
        <p>No matchups available for this user.</p>
      )}
    </div>
  );
};

export default UserProfile;