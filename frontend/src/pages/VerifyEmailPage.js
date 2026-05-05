import React, { useEffect, useRef, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { confirmEmailVerification } from '../services/api';
import '../styles/LoginPage.css';

/*
 * VerifyEmailPage — `/verify-email/:token`
 *
 * Called by the magic link in the welcome / resend email. Fires the
 * RPC immediately on mount — the user's intent was already expressed
 * by clicking the link, so there's no benefit to a second confirm
 * button.
 *
 * Three terminal states:
 *   success  → brief celebration, then redirect to /home after 2 s
 *   expired  → explain, link to resend (signed-in path) or /login
 *   used     → explain, link to /home (they're already verified)
 *
 * Reuses the login-card visual shell so all three states render
 * inside the same centred card.
 */
const VerifyEmailPage = () => {
  const { token } = useParams();
  const navigate = useNavigate();
  const [state, setState] = useState('loading'); // loading | success | expired | used | error
  // Strict-mode / re-render guard: the effect fires twice under React
  // 18's strict mode, which would double-submit the token. The token
  // is single-use server-side so the second call would flip us to
  // "used" even on a fresh link. A ref gates the RPC to one shot.
  const fired = useRef(false);

  useEffect(() => {
    if (fired.current) return;
    fired.current = true;

    let cancelled = false;
    (async () => {
      try {
        await confirmEmailVerification(token);
        if (cancelled) return;
        setState('success');
        setTimeout(() => {
          if (!cancelled) navigate('/home', { replace: true });
        }, 2000);
      } catch (err) {
        if (cancelled) return;
        const msg = (err?.response?.data?.message || '').toLowerCase();
        if (msg.includes('already used')) {
          setState('used');
        } else if (msg.includes('expired') || msg.includes('invalid')) {
          setState('expired');
        } else {
          setState('error');
        }
      }
    })();

    return () => { cancelled = true; };
  }, [token, navigate]);

  return (
    <div className="login-page">
      <div className="login-card">
        {state === 'loading' && (
          <div className="login-card__header">
            <h1 className="login-card__title">Verifying…</h1>
            <p className="login-card__subtitle">One moment.</p>
          </div>
        )}
        {state === 'success' && (
          <div className="login-card__header">
            <h1 className="login-card__title">Email verified 🎉</h1>
            <p className="login-card__subtitle">
              You're all set. Redirecting you home…
            </p>
          </div>
        )}
        {state === 'expired' && (
          <>
            <div className="login-card__header">
              <h1 className="login-card__title">Link expired</h1>
              <p className="login-card__subtitle">
                Verification links expire after 24 hours. Sign in to request
                a new one from the banner at the top of your screen.
              </p>
            </div>
            <div className="login-footer">
              <Link to="/login">Back to login</Link>
            </div>
          </>
        )}
        {state === 'used' && (
          <>
            <div className="login-card__header">
              <h1 className="login-card__title">Already verified</h1>
              <p className="login-card__subtitle">
                This link has been used — your email is already verified.
              </p>
            </div>
            <div className="login-footer">
              <Link to="/home">Go home</Link>
            </div>
          </>
        )}
        {state === 'error' && (
          <>
            <div className="login-card__header">
              <h1 className="login-card__title">Something went wrong</h1>
              <p className="login-card__subtitle">
                We couldn't verify your email just now. Try again in a moment.
              </p>
            </div>
            <div className="login-footer">
              <Link to="/login">Back to login</Link>
            </div>
          </>
        )}
      </div>
    </div>
  );
};

export default VerifyEmailPage;
