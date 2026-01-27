import React, { useEffect, useState } from 'react';
import { motion } from 'framer-motion';
import { updateMatchupItem, incrementMatchupItemVotes } from '../services/api';
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
}) => {
  const [isEditing, setIsEditing] = useState(false);
  const [itemName, setItemName] = useState(item.item ?? item.name ?? '');
  const [votes, setVotes] = useState(item.votes ?? 0);
  const [votePending, setVotePending] = useState(false);

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
    try {
      setVotePending(true);
      const res = await incrementMatchupItemVotes(item.id);
      const payload = res?.data?.response ?? res?.data ?? {};
      const updatedVotes = payload?.votes ?? payload?.Votes;
      if (typeof updatedVotes === 'number') {
        setVotes(Number(updatedVotes));
      } else {
        setVotes((prev) => prev + 1);
      }
      if (typeof onVote === 'function') {
        await onVote();
      }
    } catch (err) {
      console.error('Unable to register vote', err);
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
          onClick={computedCanEdit ? () => setIsEditing(true) : undefined}
          role={computedCanEdit ? 'button' : undefined}
          tabIndex={computedCanEdit ? 0 : undefined}
          onKeyDown={
            computedCanEdit
              ? (e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
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
          <span className="matchup-vote-score-value">{votePercent}%</span>
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

      {showVoteBar && !votingDisabled && (
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
