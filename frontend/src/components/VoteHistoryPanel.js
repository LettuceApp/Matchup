import React, { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { FiZap, FiAward } from 'react-icons/fi';
import { getUserMatchupVotes } from '../services/api';
import { relativeTime } from '../utils/time';
import '../styles/VoteHistoryPanel.css';

/*
 * VoteHistoryPanel — owner-only "matchups I've voted on" list.
 *
 * Backed by the enriched GetUserVotes RPC, which returns each pick
 * joined to its matchup title + item label + parent bracket info,
 * so the list can render presentably without N+1 fetches.
 *
 * Replaces the dropped vote_cast activity-feed kind. That kind
 * fired one "You voted for X in <matchup>" row per vote, which
 * crowded out the activity feed's receive-side signals (likes,
 * comments, mentions). The data still lives on the user's profile
 * — just behind a deliberate tab the user reaches when they
 * actually want a receipts surface.
 *
 * Owner-only by both the server gate (CodePermissionDenied for
 * non-owners) and the parent's `{isViewer && ...}` wrapper. This
 * component doesn't re-check; it trusts the mount-conditional.
 */
const VoteHistoryPanel = ({ userId, navigate }) => {
  const [votes, setVotes] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (!userId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    getUserMatchupVotes(userId)
      .then((res) => {
        if (cancelled) return;
        const list = res?.data?.votes ?? [];
        setVotes(Array.isArray(list) ? list : []);
      })
      .catch((err) => {
        if (cancelled) return;
        console.error('VoteHistoryPanel: getUserMatchupVotes', err);
        setError('Could not load your vote history.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [userId]);

  // Build the per-row link target: a bracket-child matchup routes to
  // /brackets/<id> (more contextual landing — the user sees the
  // whole bracket); a standalone matchup routes to
  // /users/<author>/matchup/<id>. Falls back to /home when the
  // join couldn't resolve an author username, which keeps the row
  // tappable instead of dead.
  const linkFor = (v) => {
    if (v.bracket_id) return `/brackets/${v.bracket_id}`;
    if (v.author_username && v.matchup_id) {
      return `/users/${v.author_username}/matchup/${v.matchup_id}`;
    }
    return '/home';
  };

  if (loading) {
    return (
      <div className="profile-tab-section">
        <header className="profile-section-header">
          <div>
            <h2>Your votes</h2>
            <p>Matchups and brackets you've picked a side in.</p>
          </div>
        </header>
        <p className="vote-history-state">Loading…</p>
      </div>
    );
  }

  if (error) {
    return (
      <div className="profile-tab-section">
        <header className="profile-section-header">
          <div>
            <h2>Your votes</h2>
            <p>Matchups and brackets you've picked a side in.</p>
          </div>
        </header>
        <p className="vote-history-state vote-history-state--error">{error}</p>
      </div>
    );
  }

  return (
    <div className="profile-tab-section">
      <header className="profile-section-header">
        <div>
          <h2>Your votes</h2>
          <p>
            {votes.length === 0
              ? "Nothing yet — vote on a matchup and it'll show up here."
              : `${votes.length} pick${votes.length === 1 ? '' : 's'} across matchups + brackets.`}
          </p>
        </div>
      </header>

      {votes.length === 0 ? (
        <div className="vote-history-empty">
          <button
            type="button"
            className="profile-secondary-button"
            onClick={() => navigate?.('/home')}
          >
            Browse matchups
          </button>
        </div>
      ) : (
        <ul className="vote-history-list">
          {votes.map((v) => {
            const title = v.matchup_title || 'a matchup';
            const pick = v.item_label || '';
            const Icon = v.bracket_id ? FiAward : FiZap;
            return (
              <li key={v.id} className="vote-history-row">
                <Link to={linkFor(v)} className="vote-history-link">
                  <span className="vote-history-icon" aria-hidden="true">
                    <Icon />
                  </span>
                  <div className="vote-history-body">
                    <p className="vote-history-title">
                      {v.bracket_id && v.bracket_title && (
                        <span className="vote-history-bracket">
                          {v.bracket_title} · {' '}
                        </span>
                      )}
                      {title}
                    </p>
                    {pick && (
                      <p className="vote-history-pick">
                        Voted <strong>{pick}</strong>
                      </p>
                    )}
                  </div>
                  <time
                    className="vote-history-time"
                    dateTime={v.created_at}
                    title={new Date(v.created_at).toLocaleString()}
                  >
                    {relativeTime(v.created_at)}
                  </time>
                </Link>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
};

export default VoteHistoryPanel;
