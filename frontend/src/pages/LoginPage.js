import React, { useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { login } from '../services/api';
import { identifyUser, track } from '../utils/analytics';
import '../styles/LoginPage.css';

const LoginPage = () => {
  const [identifier, setIdentifier] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState(null);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const from = location.state?.from?.pathname || '/home';
  // One-shot banner forwarded by /reset-password success + /settings/
  // account delete. React Router keeps location.state across renders,
  // so we read once and trust the user to refresh if they want it
  // gone (history.replaceState would clear it but also strips any
  // `from` hint we might need for redirect-after-login).
  const bannerInfo = location.state?.info;

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    try {
      setIsSubmitting(true);
      const payload = await login({ email: identifier, password });
      if (payload?.token && payload?.id) {
        // Identify before navigate so the next pageview already
        // carries the user_id. The login form's `identifier` field
        // accepts username OR email — we don't know which without
        // string-sniffing, so we pass it as `email_or_username` to
        // keep the cohort attribute honest.
        identifyUser(payload.id, {
          username: payload.username,
          email_or_username: identifier,
        });
        track('login_completed');
        // Honour the redirect-after-login hint that RequireAuth puts in
        // location.state when an unauthenticated user hits a gated route.
        // Falls back to /home when the user came here directly.
        navigate(from, { replace: true });
      } else {
        // Server returned 200 but the response is missing the access
        // token or user id — should never happen on a healthy backend,
        // but surfacing the actual payload shape helps diagnose if it
        // does (e.g. a proto-mapping regression silently strips fields).
        console.error('Login response missing token/id:', payload);
        setError('Login response missing fields. Open DevTools console for details.');
      }
    } catch (err) {
      // Surface the SPECIFIC server message instead of the generic
      // "check your credentials" line. Connect-RPC's error body shape
      // is { code, message }; axios puts it on err.response.data. If
      // the call failed at the network layer (no response), fall back
      // to err.message. Logging the full error object too so DevTools
      // has the stack + raw response when the user reports a bug.
      console.error('[login]', err);
      const serverMsg =
        err?.response?.data?.message ||
        err?.response?.data?.error ||
        err?.message ||
        '';
      const code = err?.response?.data?.code;
      const status = err?.response?.status;
      if (serverMsg) {
        setError(`Login failed: ${serverMsg}${code ? ` (${code})` : ''}`);
      } else if (status) {
        setError(`Login failed (HTTP ${status}). Try again or check DevTools.`);
      } else {
        setError('Login failed. Check your network connection and try again.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-card__header">
          <h1 className="login-card__title">Welcome back</h1>
          <p className="login-card__subtitle">
            Sign in to continue creating, voting, and sharing your matchups.
          </p>
        </div>

        {bannerInfo && <p className="login-info">{bannerInfo}</p>}
        {error && <p className="login-error">{error}</p>}

        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label htmlFor="login-identifier">Email or Username</label>
            <input
              id="login-identifier"
              type="text"
              className="login-input"
              placeholder="Enter your email or username"
              value={identifier}
              onChange={(e) => setIdentifier(e.target.value)}
              autoComplete="username"
              required
            />
          </div>
          <div className="login-field">
            <label htmlFor="login-password">Password</label>
            <input
              id="login-password"
              type="password"
              className="login-input"
              placeholder="Enter your password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
            <Link to="/forgot-password" className="login-forgot-link">
              Forgot password?
            </Link>
          </div>
          <button
            type="submit"
            className="login-primary-button"
            disabled={isSubmitting}
          >
            {isSubmitting ? 'Signing in…' : 'Login'}
          </button>
        </form>

        <div className="login-footer">
          <span>Don't have an account?</span>
          <button type="button" onClick={() => navigate('/register')}>
            Register
          </button>
        </div>

        {/* Legal footer — App Store + Play Store reviewers expect the
            sign-in surface to reference the Privacy Policy + Terms. */}
        <p className="login-legal-footer">
          By continuing, you agree to our{' '}
          <Link to="/terms">Terms</Link> and{' '}
          <Link to="/privacy">Privacy Policy</Link>.
        </p>
      </div>
    </div>
  );
};

export default LoginPage;
