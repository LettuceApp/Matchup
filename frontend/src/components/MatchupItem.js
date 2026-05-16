import React, { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import { updateMatchupItem, incrementMatchupItemVotes } from '../services/api';
import { useAnonUpgradePrompt } from '../contexts/AnonUpgradeContext';
import { track } from '../utils/analytics';
import '../styles/MatchupItem.css';

const MatchupItem = ({
  item,
  allowEdit,
  isOwner = false,
  isBracketMatchup = false,
  isWinner = false,
  hasWinner = false,
  isVotingLocked = false,
  canOverrideWinner = false,
  onOverrideWinner,
  onVote,
  disabled = false,
  totalVotes = null,
  showVoteBar = false,
  isLeading = false,
  isVoted = false,
}) => {
  const [isEditing, setIsEditing] = useState(false);
  const [itemName, setItemName] = useState(item.item ?? item.name ?? '');
  const [votes, setVotes] = useState(Number(item.votes ?? 0));
  const [votePending, setVotePending] = useState(false);
  const { promptUpgrade } = useAnonUpgradePrompt();

  useEffect(() => {
    setVotes(Number(item.votes ?? item.Votes ?? 0));
  }, [item.votes, item.Votes, item.id]);

  useEffect(() => {
    setItemName(item.item ?? item.name ?? '');
  }, [item.item, item.name, item.id]);

  const computedCanEdit =
    typeof allowEdit === 'boolean'
      ? allowEdit
      : (isOwner && !isBracketMatchup && !disabled);

  const votingDisabled = disabled || isVotingLocked || votePending;
  const showWinnerButton =
    Boolean(canOverrideWinner && typeof onOverrideWinner === 'function');

  const handleSave = async () => {
    if (!computedCanEdit) return;
    try {
      await updateMatchupItem(item.id, { item: itemName });
    } catch (err) {
      console.error('Unable to update matchup item', err);
    } finally {
      setIsEditing(false);
    }
  };

  const handleVote = async () => {
    if (votingDisabled) return;
    // Guardrail for the silent-fail case: if the item's public_id is
    // somehow missing (broken DB row, stale render after a delete,
    // user-as-item where the user got pruned), the RPC will hit a 404
    // path that surfaces nothing useful to the user. Reject early with
    // a clear message — better than a click that appears to do nothing.
    if (!item?.id) {
      console.warn('handleVote: item has no id, refusing', item);
      alert("This option can't be voted on — try refreshing the page.");
      return;
    }
    try {
      setVotePending(true);
      const res = await incrementMatchupItemVotes(item.id);
      // Log the full response shape — helps diagnose "I clicked but
      // nothing changed" reports (the AlreadyVoted branch returns the
      // same vote count, which looks identical to a no-op). Promoted
      // from console.debug to console.log so users see it without
      // having to enable Verbose log level. Scoped behind a label so
      // it's grep-friendly in the console.
      const payload = res?.data?.response ?? res?.data ?? {};
      const alreadyVoted = Boolean(payload?.already_voted ?? payload?.alreadyVoted);
      const serverVotes = payload?.item?.votes ?? payload?.item?.Votes ?? null;
      console.log('[matchup-vote]', {
        item_id: item.id,
        item_label: item.item ?? item.name,
        already_voted: alreadyVoted,
        server_votes: serverVotes,
      });
      // Special case: AlreadyVoted + server_votes === 0 is the
      // signature of the data-drift bug (vote row exists in
      // matchup_votes but the items.votes rollup is stuck at zero).
      // Surface a clear warning so the user knows it's a known
      // server-side reconciliation issue rather than a click that
      // didn't register — and the admin can run
      // `go run ./cmd/backfill_vote_counts --apply` to repair.
      if (alreadyVoted && serverVotes === 0) {
        console.warn(
          '[matchup-vote] Drift detected: server reports already_voted=true with 0 votes. ' +
          'matchup_items.votes is out of sync with matchup_votes for this row. ' +
          'Run `cmd/backfill_vote_counts --apply` server-side to repair.',
        );
      }
      // Deliberately NOT calling setVotes() from the response. The
      // optimistic local bump was the cause of the "both items show
      // 100% after re-voting" bug: the response contains only the
      // CLICKED item's new count, so on a switch-vote the parent
      // (previously voted) item's local state stayed at its old count
      // until refreshMatchup eventually updated its prop. During that
      // ~200-500ms window both items' percentage bars read full. By
      // waiting for the parent's refreshMatchup to land we get a single
      // consistent paint where both old and new percentages are right.
      // The vote-pending → onVote chain still gives immediate feedback
      // via the orange-ring transition; the percentage bar updates one
      // tick later when the refresh propagates.
      track('vote_cast', {
        matchup_id: item.matchup_id ?? item.matchupId,
        item_id: item.id,
        is_bracket: Boolean(isBracketMatchup),
        already_voted: alreadyVoted,
      });
      if (typeof onVote === 'function') {
        await onVote();
      }
    } catch (err) {
      // Anon-specific server rejections map to the upgrade modal.
      // The Connect framework returns the canonical code as a
      // `code` field (lowercase snake_case) in the response body.
      const code = err?.response?.data?.code;
      const message = (err?.response?.data?.message || '').toLowerCase();
      if (code === 'resource_exhausted' && message.includes('free vote')) {
        promptUpgrade('cap');
      } else if (code === 'permission_denied' && message.includes('bracket')) {
        promptUpgrade('bracket');
      } else if (code === 'permission_denied' && message.includes('community')) {
        // Community-scoped matchups are members-only. The backend
        // returns "join this community to vote" — surface it as a
        // friendlier alert. The matchup hero already shows the
        // "From /c/<slug>" link so the user has a clear next step.
        alert('Join this community to vote on its matchups.');
      } else if (code === 'unauthenticated' && !localStorage.getItem('token')) {
        // The legacy 401 path — server rejected an anon vote on a
        // route that doesn't yet allow them. Treat like the cap.
        promptUpgrade('cap');
      } else {
        console.error('Unable to register vote', err);
        const msg = err?.response?.data?.message || err?.response?.data?.error || 'Unable to register vote. Please try again.';
        alert(msg);
      }
    } finally {
      setVotePending(false);
    }
  };

  const resolvedTotalVotes =
    typeof totalVotes === 'number' && totalVotes > 0
      ? totalVotes
      : Math.max(Number(votes), 1);
  const votePercent = Math.min(100, Math.round((Number(votes) / resolvedTotalVotes) * 100));

  const containerClasses = [
    'matchup-item',
    showVoteBar ? 'matchup-item--visual' : '',
    (disabled || isVotingLocked) ? 'is-disabled' : '',
    isWinner ? 'matchup-item--winner' : '',
    isLeading ? 'matchup-item--leading' : '',
    hasWinner && !isWinner ? 'matchup-item--loser' : '',
    isVoted ? 'matchup-item--voted' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div
      className={containerClasses}
      onClick={showVoteBar && !votingDisabled ? handleVote : undefined}
      role={showVoteBar && !votingDisabled ? 'button' : undefined}
      tabIndex={showVoteBar && !votingDisabled ? 0 : undefined}
      onKeyDown={
        showVoteBar && !votingDisabled
          ? (e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                handleVote();
              }
            }
          : undefined
      }
    >
      {/* User-as-contender (Phase 2 of the social-loop cycle). When
          the item references a user, their avatar replaces the
          item's plain thumbnail and the @username becomes the label.
          Priority over image_url because the user's identity is the
          richer signal than whatever generic thumbnail might have
          been pre-uploaded; if both are set, user wins. */}
      {item?.user_username ? (
        <>
          {item.user_avatar_path ? (
            <img
              src={item.user_avatar_path}
              alt={item.user_username}
              className="matchup-item__thumb matchup-item__thumb--user"
              decoding="async"
              loading="lazy"
            />
          ) : (
            <span className="matchup-item__thumb matchup-item__thumb--user-fallback">
              {(item.user_username || '?').charAt(0).toUpperCase()}
            </span>
          )}
        </>
      ) : (
        /* Optional thumbnail (cycle 6c). Mounts BEFORE the text label so
            it leads visually — pairwise-comparison research is consistent
            that visual richness on contender cards improves recognizability.
            The rendered <img> uses the proto's resolved image_url; empty
            paths skip the element entirely so existing text-only cards
            stay unchanged. Decoded async so Safari doesn't block paint. */
        item?.image_url && (
          <img
            src={item.image_url}
            alt={itemName}
            className="matchup-item__thumb"
            decoding="async"
            loading="lazy"
          />
        )
      )}

      {isEditing && computedCanEdit ? (
        <input
          value={itemName}
          onChange={(e) => setItemName(e.target.value)}
          onBlur={handleSave}
          onKeyDown={(e) => e.key === 'Enter' && handleSave()}
          autoFocus
          className="matchup-input"
        />
      ) : (
        <span
          className="matchup-text"
          // stopPropagation: the parent <div> also has onClick={handleVote},
          // so without it a click on the editable label fired BOTH edit
          // mode AND a vote. Defensive even when computedCanEdit is false
          // — keeps the vote/edit semantics from ever overlapping if the
          // permissions gate is later relaxed.
          onClick={computedCanEdit ? (e) => { e.stopPropagation(); setIsEditing(true); } : undefined}
          role={computedCanEdit ? 'button' : undefined}
          tabIndex={computedCanEdit ? 0 : undefined}
          onKeyDown={
            computedCanEdit
              ? (e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    e.stopPropagation();
                    setIsEditing(true);
                  }
                }
              : undefined
          }
        >
          {itemName}
        </span>
      )}

      {!showVoteBar && (
        <p className="matchup-votes">
          Votes: <span>{votes}</span>
        </p>
      )}

      {showVoteBar && (
        <div className="matchup-vote-score">
          {/*
            Show the absolute vote count alongside the percentage so
            users see "how many" not just "what share". Percentage
            stays as the primary value (drives the bar width); count
            sits next to it as a smaller secondary number. Pluralizes
            so "1 vote" doesn't read as "1 votes".
          */}
          <span className="matchup-vote-score-value">{votePercent}%</span>
          <span className="matchup-vote-score-count">
            {votes.toLocaleString()} {votes === 1 ? 'vote' : 'votes'}
          </span>
          {isLeading && !hasWinner && (
            <span className="matchup-vote-score-label">Leading</span>
          )}
        </div>
      )}

      {showVoteBar && (
        <div className="matchup-vote-bar">
          <div
            className="matchup-vote-bar-fill"
            style={{ width: `${votePercent}%` }}
          />
        </div>
      )}

      {showVoteBar && !votingDisabled && !isVoted && (
        <span className="matchup-vote-hint">Tap to vote</span>
      )}

      {!showVoteBar && (
        <div className="matchup-item-actions">
          <motion.button
            type="button"
            className="matchup-button"
            onClick={handleVote}
            disabled={votingDisabled}
            whileTap={votingDisabled ? undefined : { scale: 0.96 }}
          >
            {votePending ? 'Voting...' : '+1 Vote'}
          </motion.button>

          {showWinnerButton && (
            <button
              type="button"
              className={`matchup-override-button ${
                isWinner ? 'matchup-override-button--active' : ''
              }`}
              onClick={onOverrideWinner}
            >
              {isWinner ? 'Winner selected' : 'Select winner'}
            </button>
          )}
        </div>
      )}

      {isWinner && (
        <span className="matchup-winner-badge">Winner</span>
      )}
    </div>
  );
};

export default MatchupItem;
