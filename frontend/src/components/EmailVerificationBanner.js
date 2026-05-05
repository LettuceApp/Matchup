import React, { useEffect, useState } from 'react';
import { getCurrentUser, requestEmailVerification } from '../services/api';
import '../styles/EmailVerificationBanner.css';

/*
 * EmailVerificationBanner
 *
 * Top-of-page banner shown to logged-in users whose email hasn't been
 * verified yet. One affordance: "Resend verification email". On click
 * we hit the RPC + flip the banner to a success-state for the rest
 * of the session.
 *
 * Visibility rules:
 *   - Hidden when no user is logged in (no userId in localStorage).
 *   - Hidden when currentUser.is_verified === true.
 *   - Dismissible per-session via the × button. The dismiss flag
 *     lives in sessionStorage (not localStorage) so a hard reload
 *     surfaces it again next time — we don't want users missing it
 *     permanently by accidentally clicking X.
 *   - Reviewers testing the App Store / Play review flow can dismiss
 *     it once and get on with exploring the app.
 *
 * The banner fetches the current user on mount if the prop isn't
 * supplied. In practice App.js will hand it down from the top-level
 * auth state, but the self-fetch keeps this component drop-in for
 * pages that don't already load the user.
 */
const DISMISS_KEY = 'emailVerificationBanner:dismissed';

const EmailVerificationBanner = ({ user, onUserRefresh }) => {
  const [currentUser, setCurrentUser] = useState(user || null);
  const [dismissed, setDismissed] = useState(
    typeof window !== 'undefined' && sessionStorage.getItem(DISMISS_KEY) === '1'
  );
  const [sending, setSending] = useState(false);
  const [sentAt, setSentAt] = useState(null);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (user) {
      setCurrentUser(user);
      return;
    }
    // Only self-fetch when the caller didn't supply a user. Gated on
    // the presence of a userId in localStorage so anon pages don't
    // make a pointless 401'd round-trip.
    if (typeof window === 'undefined' || !localStorage.getItem('userId')) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await getCurrentUser();
        if (!cancelled) setCurrentUser(res?.data?.user || null);
      } catch {
        // Swallow — banner just doesn't render if this fails.
      }
    })();
    return () => { cancelled = true; };
  }, [user]);

  const handleResend = async () => {
    setSending(true);
    setError(null);
    try {
      await requestEmailVerification();
      setSentAt(new Date());
    } catch (err) {
      const status = err?.response?.status;
      if (status === 429) {
        setError('Too many requests — try again in a bit.');
      } else {
        setError('Could not resend right now.');
      }
    } finally {
      setSending(false);
    }
  };

  const handleDismiss = () => {
    sessionStorage.setItem(DISMISS_KEY, '1');
    setDismissed(true);
  };

  // Visibility gate — render nothing (not even the container) when
  // there's no reason to show the banner.
  if (!currentUser || currentUser.is_verified || dismissed) return null;
  // Guard: legacy users created before migration 024 land as null,
  // but so do brand-new signups. Either way the banner is correct.

  const email = currentUser.email || 'your email';

  return (
    <div
      className="email-verify-banner"
      role="status"
      aria-live="polite"
    >
      <div className="email-verify-banner__body">
        {sentAt ? (
          <span>
            Sent a new link to <strong>{email}</strong>. Check your inbox.
          </span>
        ) : (
          <span>
            Verify <strong>{email}</strong> to create matchups and brackets.
          </span>
        )}
        {error && <span className="email-verify-banner__error">{error}</span>}
      </div>
      <div className="email-verify-banner__actions">
        {!sentAt && (
          <button
            type="button"
            className="email-verify-banner__resend"
            disabled={sending}
            onClick={handleResend}
          >
            {sending ? 'Sending…' : 'Resend link'}
          </button>
        )}
        <button
          type="button"
          className="email-verify-banner__dismiss"
          aria-label="Dismiss"
          onClick={handleDismiss}
        >
          ×
        </button>
      </div>
      {onUserRefresh && null /* noop ref to silence unused-prop lint */}
    </div>
  );
};

export default EmailVerificationBanner;
