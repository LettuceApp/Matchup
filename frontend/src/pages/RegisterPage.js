import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { createUser } from '../services/api';

const RegisterPage = () => {
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const navigate = useNavigate();

  useEffect(() => {
    // If a token exists in localStorage, redirect to the home page
    const token = localStorage.getItem('token');
    if (token) {
      navigate('/');
    }
  }, [navigate]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    try {
      // Make API call to create a new user
      const response = await createUser({ email, username, password });
      console.log('Registration successful:', response.data);

      // Automatically log in the user and redirect to the home page
      localStorage.setItem('token', response.data.token);
      localStorage.setItem('userId', response.data.id);
      navigate('/');
    } catch (err) {
      console.error('Registration error:', err.response ? err.response.data : err.message);
      alert('Registration failed. Please try again.');
    }
  };

  return (
    <div>
      <h1>Register</h1>
      <form onSubmit={handleSubmit}>
        <input
          type="email"
          placeholder="Email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          required
        />
        <input
          type="text"
          placeholder="Username"
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          required
        />
        <input
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          required
        />
        <button type="submit">Register</button>
      </form>
      <div style={{ marginTop: '20px' }}>
        <p>Already have an account?</p>
        <button onClick={() => navigate('/login')}>Login</button>
      </div>
    </div>
  );
};

export default RegisterPage;
