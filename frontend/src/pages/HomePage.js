import React, { useState, useEffect } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { getUserMatchups } from '../services/api';
import NavigationBar from '../components/NavigationBar'; // Import NavigationBar component
import Button from '../components/Button'; // Import Button component

const HomePage = () => {
  const [matchups, setMatchups] = useState([]);
  const navigate = useNavigate();

  useEffect(() => {
    const fetchMatchups = async () => {
      // Check if token is available in localStorage
      const token = localStorage.getItem('token');
      if (!token) {
        console.error('Token not found, redirecting to login page...');
        navigate('/login');
        return;
      }

      try {
        // Get user ID from localStorage
        const userId = localStorage.getItem('userId');
        if (!userId) {
          console.error('User ID not found');
          return;
        }

        // Fetch user-specific matchups
        const response = await getUserMatchups(userId);
        console.log('API Response:', response.data); // Log the entire response to see its structure

        // Set matchups state depending on the response structure
        const matchupsData = response.data.response || response.data;
        console.log('Matchups Data:', matchupsData); // Log matchups data before setting state
        setMatchups(matchupsData);
      } catch (err) {
        console.error('Failed to fetch user matchups:', err);
      }
    };
    fetchMatchups();
  }, [navigate]);

  // Navigate to the CreateMatchup page
  const navigateToCreateMatchup = () => {
    navigate('/create-matchup');
  };

  return (
    <div>
      <NavigationBar /> {/* Include NavigationBar at the top */}

      <h1>Your Matchups</h1>
      {matchups.length > 0 ? (
        matchups.map((matchup) => {
          const userId = localStorage.getItem('userId'); // Get user ID from localStorage
          return (
            <div key={matchup.id}>
              <h2>
                <Link to={`/users/${userId}/matchup/${matchup.id}`}>{matchup.title}</Link> {/* Make title clickable */}
              </h2>
              <p>{matchup.description}</p>
            </div>
          );
        })
      ) : (
        <p>No matchups available.</p>
      )}
      
      {/* Create a Matchup Button */}
      <Button onClick={navigateToCreateMatchup}>Create a Matchup</Button>
    </div>
  );
};

export default HomePage;