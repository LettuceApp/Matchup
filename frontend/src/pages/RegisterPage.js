import React, { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { createUser, login } from '../services/api';
import { clearAnonId, peekAnonId } from '../utils/anonId';
import { identifyUser, track } from '../utils/analytics';
import '../styles/RegisterPage.css';

const RegisterPage = () => {
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const from = location.state?.from?.pathname || '/home';

  // Top-of-funnel signal: somebody made it to the signup page.
  // Combined with `signup_completed` below, this gives us the
  // form-abandonment rate (fired-once-per-mount, so a refresh
  // counts as a fresh attempt).
  useEffect(() => {
    track('signup_started');
  }, []);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    try {
      setIsSubmitting(true);
      // createUser sends X-Anon-Id automatically (axios interceptor),
      // and the server merges any anon votes into the new account.
      await createUser({ email, username, password });

      // Login also re-runs mergeDeviceVotesToUser — defence in depth
      // in case createUser's merge missed any rows due to ordering.
      const payload = await login({ email, password });

      if (payload?.token && payload?.id) {
        // Capture the anon-id BEFORE clearAnonId wipes it — we want
        // it threaded into the identify call so PostHog can stitch
        // the pre-signup anon events to the new user_id. Without
        // this, the conversion funnel can't resolve the path.
        const carriedAnonId = peekAnonId();

        identifyUser(payload.id, {
          username: payload.username,
          email,
          anon_id: carriedAnonId || undefined,
          signup_at: new Date().toISOString(),
        });
        track('signup_completed', {
          had_anon_id: Boolean(carriedAnonId),
        });

        // Anon ID has done its job (votes now live on the user
        // account). Clearing it prevents the next anon-style
        // request from accidentally re-running the merge against a
        // stale ID after sign-out.
        clearAnonId();

        const dest = payload.username
          ? `/users/${payload.username}`
          : '/home';
        navigate(dest, { replace: true });
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

        {/* Legal footer — the store-reviewer-expected reference to
            Terms + Privacy on the signup surface. */}
        <p className="register-legal-footer">
          By creating an account, you agree to our{' '}
          <Link to="/terms">Terms</Link> and{' '}
          <Link to="/privacy">Privacy Policy</Link>.
        </p>
      </div>
    </div>
  );
};

export default RegisterPage;
