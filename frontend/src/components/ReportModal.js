import React, { useState } from 'react';
import { reportContent } from '../services/api';
import '../styles/ReportModal.css';

/*
 * ReportModal — single dialog used across matchup / bracket / comment /
 * user profile surfaces.
 *
 * Props:
 *   subjectType  "matchup" | "bracket" | "comment" | "bracket_comment" | "user"
 *   subjectId    UUID public_id of the reported entity
 *   onClose()    caller-supplied dismiss handler; fires on success
 *                AND cancel (caller decides whether to show a toast)
 *
 * The modal is deliberately one file with no router dependency so any
 * page can conditionally render it without route coordination. Every
 * reason option matches the closed vocabulary the server enforces; a
 * drift between the two flips the user into a CodeInvalidArgument.
 */

const REASONS = [
  { value: 'harassment',     label: 'Harassment or bullying' },
  { value: 'spam',           label: 'Spam or misleading' },
  { value: 'violence',       label: 'Violence or threats' },
  { value: 'nudity',         label: 'Nudity or sexual content' },
  { value: 'misinformation', label: 'Misinformation' },
  { value: 'self_harm',      label: 'Self-harm or suicide' },
  { value: 'copyright',      label: 'Copyright or IP violation' },
  { value: 'other',          label: 'Other (please describe)' },
];

const ReportModal = ({ subjectType, subjectId, onClose }) => {
  const [reason, setReason] = useState('harassment');
  const [detail, setDetail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [sent, setSent] = useState(false);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);
    // Client-side guard mirrors the server's "other needs detail" rule.
    if (reason === 'other' && !detail.trim()) {
      setError('Please describe why you’re reporting this.');
      return;
    }
    setSubmitting(true);
    try {
      await reportContent({
        subjectType,
        subjectId,
        reason,
        reasonDetail: detail.trim() || undefined,
      });
      setSent(true);
    } catch (err) {
      const status = err?.response?.status;
      if (status === 429) {
        setError('You’re reporting too fast — try again in a little while.');
      } else {
        setError('Could not submit the report. Try again in a moment.');
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="report-modal__scrim" role="dialog" aria-modal="true" onMouseDown={(e) => {
      // Click on backdrop (not inside the card) dismisses.
      if (e.target === e.currentTarget) onClose?.();
    }}>
      <div className="report-modal__card">
        {sent ? (
          <>
            <h2 className="report-modal__title">Thanks — we’ll review this.</h2>
            <p className="report-modal__body">
              Our moderation team looks at every report. You won’t hear back
              about this one specifically, but we appreciate the heads-up.
            </p>
            <div className="report-modal__actions">
              <button type="button" className="report-modal__primary" onClick={onClose}>
                Close
              </button>
            </div>
          </>
        ) : (
          <form onSubmit={handleSubmit}>
            <h2 className="report-modal__title">Report this content</h2>
            <p className="report-modal__body">
              Let us know what’s wrong. Reports are confidential.
            </p>

            <label className="report-modal__label">
              <span>Why are you reporting this?</span>
              <select value={reason} onChange={(e) => setReason(e.target.value)}>
                {REASONS.map((r) => (
                  <option key={r.value} value={r.value}>{r.label}</option>
                ))}
              </select>
            </label>

            <label className="report-modal__label">
              <span>
                {reason === 'other' ? 'Please describe (required)' : 'Additional detail (optional)'}
              </span>
              <textarea
                rows={3}
                value={detail}
                onChange={(e) => setDetail(e.target.value)}
                placeholder="What exactly should we look at?"
              />
            </label>

            {error && <p className="report-modal__error">{error}</p>}

            <div className="report-modal__actions">
              <button
                type="button"
                className="report-modal__cancel"
                onClick={onClose}
                disabled={submitting}
              >
                Cancel
              </button>
              <button
                type="submit"
                className="report-modal__primary"
                disabled={submitting}
              >
                {submitting ? 'Sending…' : 'Submit report'}
              </button>
            </div>
          </form>
        )}
      </div>
    </div>
  );
};

export default ReportModal;
