import React, { useCallback, useEffect, useState } from 'react';
import { adminListReports, adminResolveReport } from '../services/api';

/*
 * AdminReports — moderation queue embedded in the Admin Dashboard.
 *
 * Filters by status (open / resolved). Each row surfaces the key
 * context (who reported, what subject, why) + one of four resolution
 * actions — dismiss, remove content, warn, or ban. Ban prompts for a
 * reason since it mirrors into users.ban_reason + the audit log.
 *
 * Only drops into the admin dashboard shell; no route of its own.
 */

const REASON_LABELS = {
  harassment: 'Harassment',
  spam: 'Spam',
  violence: 'Violence',
  nudity: 'Nudity',
  misinformation: 'Misinformation',
  self_harm: 'Self-harm',
  copyright: 'Copyright',
  other: 'Other',
};

const SUBJECT_LABELS = {
  matchup: 'Matchup',
  bracket: 'Bracket',
  comment: 'Matchup comment',
  bracket_comment: 'Bracket comment',
  user: 'User profile',
};

const AdminReports = () => {
  const [status, setStatus] = useState('open');
  const [reports, setReports] = useState([]);
  const [cursor, setCursor] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [busyId, setBusyId] = useState(null);

  const load = useCallback(async (filterStatus, pagingCursor) => {
    setLoading(true);
    setError(null);
    try {
      const res = await adminListReports({
        status: filterStatus,
        ...(pagingCursor ? { cursor: pagingCursor } : {}),
      });
      const payload = res?.data?.response || res?.data || {};
      const incoming = payload.reports || [];
      setReports((prev) => (pagingCursor ? [...prev, ...incoming] : incoming));
      setCursor(payload.next_cursor || null);
    } catch (err) {
      console.error('adminListReports', err);
      setError('Unable to load the moderation queue.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load(status);
  }, [status, load]);

  const resolve = async (report, resolution) => {
    // Ban prompts for a short reason; everything else is one-click.
    let banReason;
    let notes;
    if (resolution === 'ban_user') {
      banReason = window.prompt(
        `Ban the author of this ${SUBJECT_LABELS[report.subject_type] || report.subject_type}?\nEnter a short reason (shown in the audit log + mirrored to the account record):`
      );
      if (!banReason || !banReason.trim()) return;
    } else if (resolution === 'warn_user') {
      notes = window.prompt(
        'Optional: note to attach to the warning record.',
        ''
      );
    }

    setBusyId(report.id);
    setError(null);
    try {
      await adminResolveReport({
        reportId: report.id,
        resolution,
        banReason: banReason || undefined,
        notes: notes || undefined,
      });
      // Drop the row from the current view since it's no longer open.
      setReports((prev) => prev.filter((r) => r.id !== report.id));
    } catch (err) {
      console.error('resolve report', err);
      setError('Action failed. Try again.');
    } finally {
      setBusyId(null);
    }
  };

  return (
    <section className="admin-section">
      <div className="admin-section-header">
        <div>
          <h2>Reports</h2>
          <p>Review user-submitted reports and take action.</p>
        </div>
        <div className="admin-filter-pills">
          <button
            type="button"
            className={`admin-filter-pill ${status === 'open' ? 'admin-filter-pill--active' : ''}`}
            onClick={() => setStatus('open')}
          >
            Open
          </button>
          <button
            type="button"
            className={`admin-filter-pill ${status === 'resolved' ? 'admin-filter-pill--active' : ''}`}
            onClick={() => setStatus('resolved')}
          >
            Resolved
          </button>
        </div>
      </div>

      {loading && <div className="admin-status-card">Loading reports…</div>}
      {error && !loading && (
        <div className="admin-status-card admin-status-card--error">{error}</div>
      )}

      {!loading && !error && (
        <>
          <table className="admin-table">
            <thead>
              <tr>
                <th>Reporter</th>
                <th>Subject</th>
                <th>Reason</th>
                <th>Preview</th>
                <th>Created</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {reports.length === 0 && (
                <tr>
                  <td colSpan="6" className="admin-empty">
                    {status === 'open'
                      ? 'No open reports. Great work.'
                      : 'No resolved reports yet.'}
                  </td>
                </tr>
              )}
              {reports.map((r) => (
                <tr key={r.id}>
                  <td>{r.reporter_username || `#${r.reporter_id}`}</td>
                  <td>
                    <div className="admin-report-subject">
                      <span className="admin-report-type">
                        {SUBJECT_LABELS[r.subject_type] || r.subject_type}
                      </span>
                      <code className="admin-report-id">{r.subject_id}</code>
                    </div>
                  </td>
                  <td>
                    <div>
                      <strong>{REASON_LABELS[r.reason] || r.reason}</strong>
                    </div>
                    {r.reason_detail && (
                      <div className="admin-report-detail">{r.reason_detail}</div>
                    )}
                  </td>
                  <td className="admin-report-preview">
                    {r.subject_preview || <em>(no preview)</em>}
                  </td>
                  <td>{new Date(r.created_at).toLocaleString()}</td>
                  <td>
                    {status === 'open' ? (
                      <div className="admin-action-group">
                        <button
                          type="button"
                          className="admin-link"
                          disabled={busyId === r.id}
                          onClick={() => resolve(r, 'dismiss')}
                        >
                          Dismiss
                        </button>
                        <button
                          type="button"
                          className="admin-link"
                          disabled={busyId === r.id}
                          onClick={() => resolve(r, 'warn_user')}
                        >
                          Warn
                        </button>
                        <button
                          type="button"
                          className="admin-link admin-link--danger"
                          disabled={busyId === r.id}
                          onClick={() => resolve(r, 'remove_content')}
                        >
                          Remove content
                        </button>
                        <button
                          type="button"
                          className="admin-link admin-link--danger"
                          disabled={busyId === r.id}
                          onClick={() => resolve(r, 'ban_user')}
                        >
                          Ban user
                        </button>
                      </div>
                    ) : (
                      <span className="admin-report-resolution">
                        {r.resolution}
                        {r.reviewed_by_username && ` · ${r.reviewed_by_username}`}
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {cursor && (
            <button
              type="button"
              className="admin-link"
              onClick={() => load(status, cursor)}
            >
              Load more
            </button>
          )}
        </>
      )}
    </section>
  );
};

export default AdminReports;
