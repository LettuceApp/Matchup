import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { forgotPassword } from '../services/api';
import '../styles/LoginPage.css';

/*
 * ForgotPasswordPage — `/forgot-password`
 *
 * Single email field. Submit fires the RPC and then flips to a
 * neutral success panel regardless of whether the email was
 * recognised. The server returns the same response for both cases
 * (account-existence leak defence), so mirroring that on the client
 * keeps behaviour consistent — and prevents a "hmm, that email is
 * registered" timing tell if we ever added client-side branching.
 *
 * Reuses the `login-*` CSS classes so the card visuals match the
 * sign-in screen — no new stylesheet needed.
 */
const ForgotPasswordPage = () => {
  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState(null);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    if (!email.trim()) {
      setError('Please enter the email for your account.');
      return;
    }
    setSubmitting(true);
    try {
      await forgotPassword({ email: email.trim() });
      setSent(true);
    } catch (err) {
      // Backend responds 200 for unknown emails, so any error here is
      // genuinely broken plumbing — likely a network or validation
      // fault. Surface a soft message without revealing account state.
      const msg = err?.response?.data?.message || '';
      if (msg.toLowerCase().includes('email')) {
        setError('Please enter a valid email address.');
      } else {
        setError('Something went wrong. Try again in a moment.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="login-page">
      <div className="login-card">
        {sent ? (
          <>
            <div className="login-card__header">
              <h1 className="login-card__title">Check your email</h1>
              <p className="login-card__subtitle">
                If that email is registered with Matchup, we’ve sent a reset link.
                It expires in 2 hours.
              </p>
            </div>
            <div className="login-footer">
              <Link to="/login">Back to login</Link>
            </div>
          </>
        ) : (
          <>
            <div className="login-card__header">
              <h1 className="login-card__title">Forgot password?</h1>
              <p className="login-card__subtitle">
                Enter the email you signed up with and we’ll send a reset link.
              </p>
            </div>

            {error && <p className="login-error">{error}</p>}

            <form onSubmit={handleSubmit} className="login-form">
              <div className="login-field">
                <label htmlFor="forgot-email">Email</label>
                <input
                  id="forgot-email"
                  type="email"
                  className="login-input"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  autoComplete="email"
                  autoFocus
                  required
                />
              </div>
              <button
                type="submit"
                className="login-primary-button"
                disabled={submitting}
              >
                {submitting ? 'Sending…' : 'Send reset link'}
              </button>
            </form>

            <div className="login-footer">
              <Link to="/login">Back to login</Link>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default ForgotPasswordPage;
