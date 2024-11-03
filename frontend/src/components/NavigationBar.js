import React from 'react';
import { useNavigate } from 'react-router-dom';

const NavigationBar = () => {
  const navigate = useNavigate();

  // Handle Logout - clear local storage and redirect to login
  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    navigate('/login'); // Redirect to login page
  };

  return (
    <nav style={{ padding: '10px', backgroundColor: '#f0f0f0', marginBottom: '20px' }}>
      <button onClick={() => navigate('/')}>Home</button>
      <button onClick={handleLogout} style={{ marginLeft: '10px' }}>Logout</button>
    </nav>
  );
};

export default NavigationBar;
