import React, { useState } from 'react';
import { updateMatchupItem, incrementMatchupItemVotes } from '../services/api';
import '../styles/MatchupItem.css';

const MatchupItem = ({ item, isOwner, refreshItems }) => {
  const [isEditing, setIsEditing] = useState(false);
  const [itemName, setItemName] = useState(item.item);
  const [votes, setVotes] = useState(item.votes);

  const handleEditClick = () => {
    if (isOwner) setIsEditing(true);
  };

  const handleSaveClick = async () => {
    try {
      await updateMatchupItem(item.id, { item: itemName });
      setIsEditing(false);
      refreshItems();
    } catch (err) {
      console.error('Failed to update item:', err);
    }
  };

  const handleIncrement = async () => {
    try {
      await incrementMatchupItemVotes(item.id);
      setVotes(votes + 1);
    } catch (err) {
      console.error('Failed to increment votes:', err);
    }
  };

  return (
    <div className="matchup-item">
      {isEditing ? (
        <input
          value={itemName}
          onChange={(e) => setItemName(e.target.value)}
          onBlur={handleSaveClick}
          className="matchup-input"
        />
      ) : (
        <span onClick={handleEditClick} className="matchup-text">
          {itemName}
        </span>
      )}
      <p className="matchup-votes">Votes: {votes}</p>
      <button onClick={handleIncrement} className="matchup-button">+1 Vote</button>
    </div>
  );
};

export default MatchupItem;
