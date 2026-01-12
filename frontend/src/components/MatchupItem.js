import React, { useEffect, useState } from 'react';
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

  const containerClasses = [
    'matchup-item',
    (disabled || isVotingLocked) ? 'is-disabled' : '',
    isWinner ? 'matchup-item--winner' : '',
    hasWinner && !isWinner ? 'matchup-item--loser' : '',
  ]
    .filter(Boolean)
    .join(' ');

  return (
    <div className={containerClasses}>
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

      <p className="matchup-votes">
        Votes: <span>{votes}</span>
      </p>

      <div className="matchup-item-actions">
        <button
          type="button"
          className="matchup-button"
          onClick={handleVote}
          disabled={votingDisabled}
        >
          {votePending ? 'Voting…' : '+1 Vote'}
        </button>

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

      {isWinner && (
        <span className="matchup-winner-badge">Winner</span>
      )}
    </div>
  );
};

export default MatchupItem;
