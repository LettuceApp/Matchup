import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import { deleteMyAccount, signOutLocally } from '../services/api';
import '../styles/AccountSettings.css';

/*
 * AccountSettings — `/settings/account`
 *
 * Currently hosts just the "Delete my account" flow. Future additions
 * (change password, change email, download-my-data) land on this
 * page to avoid scattering destructive-action UI across the profile.
 *
 * Delete flow:
 *   1. User clicks "Delete my account" → confirmation card opens.
 *   2. User re-enters current password + optional reason.
 *   3. On submit, call DeleteMyAccount → server soft-deletes +
 *      revokes this device's refresh token.
 *   4. Clear local auth state, redirect to /login with a banner
 *      that confirms the 30-day retention window.
 */

const AccountSettings = () => {
  const navigate = useNavigate();
  const [showConfirm, setShowConfirm] = useState(false);
  const [password, setPassword] = useState('');
  const [reason, setReason] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (!password) {
      setError('Please enter your current password to confirm.');
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const res = await deleteMyAccount(password, reason.trim() || undefined);
      const hardDeleteAt = res?.data?.hard_delete_at;
      // Clear every auth-related localStorage key + the axios default
      // Authorization header. Server already revoked the refresh
      // token; this clears the client side too.
      signOutLocally();
      navigate('/login', {
        replace: true,
        state: {
          info: hardDeleteAt
            ? `Your account is deactivated. It will be fully deleted on ${new Date(hardDeleteAt).toLocaleDateString()}.`
            : 'Your account is deactivated.',
        },
      });
    } catch (err) {
      // Server surfaces CodeUnauthenticated for wrong-password; any
      // other error is system-side. Keep the message friendly.
      const msg = err?.response?.data?.message || '';
      if (msg.toLowerCase().includes('password')) {
        setError('Password is incorrect.');
      } else {
        setError('Could not delete your account. Try again in a moment.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <>
      <NavigationBar />
      <div className="account-settings">
        <div className="account-settings__content">
          <h1>Account settings</h1>

          <section className="account-settings__section account-settings__danger">
            <header>
              <h2>Delete account</h2>
              <p>
                This deactivates your account immediately and permanently deletes
                your data after 30 days. Your matchups, comments, and votes will
                be anonymized to <em>[deleted]</em> in the meantime.
              </p>
            </header>

            {!showConfirm ? (
              <button
                type="button"
                className="account-settings__danger-button"
                onClick={() => setShowConfirm(true)}
              >
                Delete my account
              </button>
            ) : (
              <form className="account-settings__confirm" onSubmit={handleSubmit}>
                <label>
                  <span>Current password</span>
                  <input
                    type="password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    autoFocus
                    autoComplete="current-password"
                  />
                </label>
                <label>
                  <span>Reason (optional)</span>
                  <textarea
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                    rows={3}
                    placeholder="Tell us why — it helps us improve."
                  />
                </label>
                {error && <p className="account-settings__error">{error}</p>}
                <div className="account-settings__actions">
                  <button
                    type="button"
                    className="account-settings__cancel"
                    onClick={() => {
                      setShowConfirm(false);
                      setPassword('');
                      setReason('');
                      setError(null);
                    }}
                    disabled={submitting}
                  >
                    Cancel
                  </button>
                  <button
                    type="submit"
                    className="account-settings__danger-button"
                    disabled={submitting}
                  >
                    {submitting ? 'Deleting…' : 'Delete permanently in 30 days'}
                  </button>
                </div>
              </form>
            )}
          </section>
        </div>
      </div>
    </>
  );
};

export default AccountSettings;
