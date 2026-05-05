import React, { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { resetPassword } from '../services/api';
import '../styles/LoginPage.css';

/*
 * ResetPasswordPage — `/reset-password/:token`
 *
 * The token comes from the emailed link. Two password fields + a
 * server round-trip. On success we redirect to /login with a success
 * banner; on invalid/expired token we keep the user on the page and
 * suggest they request a new link.
 *
 * The server enforces:
 *   - Token must exist + be less than 2 hours old
 *   - Both password fields must match
 *   - Password must be at least 6 characters
 *
 * We mirror the length check client-side purely for faster feedback;
 * the server is the source of truth.
 */
const ResetPasswordPage = () => {
  const { token } = useParams();
  const navigate = useNavigate();
  const [password, setPassword] = useState('');
  const [confirm, setConfirm] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [expired, setExpired] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    if (password.length < 6) {
      setError('Password must be at least 6 characters.');
      return;
    }
    if (password !== confirm) {
      setError('Passwords don’t match.');
      return;
    }

    setSubmitting(true);
    try {
      await resetPassword({
        token,
        new_password: password,
        retype_password: confirm,
      });
      navigate('/login', {
        replace: true,
        state: { info: 'Password reset. Sign in with your new password.' },
      });
    } catch (err) {
      const msg = (err?.response?.data?.message || '').toLowerCase();
      // Server collapses "missing token" and "too old" into a single
      // InvalidArgument with "invalid or expired" — route that to the
      // dedicated expired-link panel so the user has a clear next
      // action (request a new link).
      if (msg.includes('expired') || msg.includes('invalid')) {
        setExpired(true);
      } else if (msg.includes('password')) {
        setError('Password did not meet the requirements. Try a longer one.');
      } else {
        setError('Something went wrong. Try again in a moment.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  if (expired) {
    return (
      <div className="login-page">
        <div className="login-card">
          <div className="login-card__header">
            <h1 className="login-card__title">Link expired</h1>
            <p className="login-card__subtitle">
              This reset link is no longer valid. They expire 2 hours after
              being sent — request a fresh one.
            </p>
          </div>
          <div className="login-footer">
            <Link to="/forgot-password">Request a new reset link</Link>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="login-card__header">
          <h1 className="login-card__title">Reset password</h1>
          <p className="login-card__subtitle">
            Pick a new password. You’ll stay signed out on other devices.
          </p>
        </div>

        {error && <p className="login-error">{error}</p>}

        <form onSubmit={handleSubmit} className="login-form">
          <div className="login-field">
            <label htmlFor="reset-password">New password</label>
            <input
              id="reset-password"
              type="password"
              className="login-input"
              placeholder="At least 6 characters"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              autoFocus
              required
              minLength={6}
            />
          </div>
          <div className="login-field">
            <label htmlFor="reset-confirm">Confirm password</label>
            <input
              id="reset-confirm"
              type="password"
              className="login-input"
              placeholder="Re-enter new password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              autoComplete="new-password"
              required
              minLength={6}
            />
          </div>
          <button
            type="submit"
            className="login-primary-button"
            disabled={submitting}
          >
            {submitting ? 'Resetting…' : 'Reset password'}
          </button>
        </form>

        <div className="login-footer">
          <Link to="/login">Back to login</Link>
        </div>
      </div>
    </div>
  );
};

export default ResetPasswordPage;
