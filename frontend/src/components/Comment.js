import React from 'react';
import { Link } from 'react-router-dom';
import Button from './Button';
import { deleteComment } from '../services/api';

const Comment = ({ comment, refreshComments }) => {
  // Get the logged-in user's id from localStorage and convert to a number
  const storedUserId = localStorage.getItem('userId');
  const userId = storedUserId ? parseInt(storedUserId, 10) : null;

  // Only allow deletion if the comment's user_id matches the logged in user's id
  const isOwner = comment.user_id === userId;

  const handleDelete = async () => {
    try {
      await deleteComment(comment.id);
      if (refreshComments) refreshComments();
    } catch (error) {
      console.error('Error deleting comment:', error);
      alert('Failed to delete comment.');
    }
  };

  return (
    <div style={{ marginBottom: '10px', padding: '10px', borderBottom: '1px solid #ddd' }}>
      {/* Username */}
      <div>
        <Link
          to={`/users/${comment.user_id}/profile`}
          style={{ fontSize: '18px', fontWeight: 'bold', textDecoration: 'none', color: '#3b5998' }}
        >
          {comment.username}
        </Link>
      </div>
      {/* Comment Body */}
      <div style={{ marginTop: '5px', fontSize: '14px', color: '#333' }}>
        {comment.body}
      </div>
      {/* Timestamp and Delete Button */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginTop: '8px',
          fontSize: '12px',
          color: '#555'
        }}
      >
        <span>{new Date(comment.created_at).toLocaleString()}</span>
        {isOwner && (
          <Button
            onClick={handleDelete}
            style={{
              background: 'transparent',
              border: 'none',
              color: 'red',
              cursor: 'pointer'
            }}
          >
            Delete
          </Button>
        )}
      </div>
    </div>
  );
};

export default Comment;
