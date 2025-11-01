import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { createUser, login } from '../services/api';
import './RegisterPage.css';

const RegisterPage = () => {
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    if (localStorage.getItem('token')) {
      navigate('/');
    }
  }, [navigate]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    try {
      setIsSubmitting(true);
      await createUser({ email, username, password });

      const res = await login({ email, password });
      const payload = res.data.response || res.data;
      const token = payload?.token;
      const userId = payload?.id;

      if (token && userId) {
        localStorage.setItem('token', token);
        localStorage.setItem('userId', userId);
        navigate('/');
      } else {
        setError('We could not complete registration. Please try again.');
      }
    } catch (err) {
      console.error('Registration/login error:', err.response?.data || err.message);
      setError(
        err.response?.data?.error ||
          err.response?.data?.message ||
          'Registration failed. Please try again.'
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="register-page">
      <div className="register-card">
        <div className="register-card__header">
          <h1 className="register-card__title">Join Matchup Hub</h1>
          <p className="register-card__subtitle">
            Create an account to start crafting and sharing your head-to-head matchups.
          </p>
        </div>

        {error && <p className="register-error">{error}</p>}

        <form onSubmit={handleSubmit} className="register-form">
          <div className="register-field">
            <label htmlFor="register-email">Email</label>
            <input
              id="register-email"
              type="email"
              className="register-input"
              placeholder="your@email.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="email"
              required
            />
          </div>

          <div className="register-field">
            <label htmlFor="register-username">Username</label>
            <input
              id="register-username"
              className="register-input"
              placeholder="Pick a display name"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              required
            />
          </div>

          <div className="register-field">
            <label htmlFor="register-password">Password</label>
            <input
              id="register-password"
              type="password"
              className="register-input"
              placeholder="Create a secure password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              required
            />
          </div>

          <button
            type="submit"
            className="register-primary-button"
            disabled={isSubmitting}
          >
            {isSubmitting ? 'Creating account…' : 'Register & Login'}
          </button>
        </form>

        <div className="register-footer">
          <span>Already have an account?</span>
          <button type="button" onClick={() => navigate('/login')}>
            Log in
          </button>
        </div>
      </div>
    </div>
  );
};

export default RegisterPage;
