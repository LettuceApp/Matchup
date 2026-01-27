import React from 'react';
import { Link } from 'react-router-dom';
import { deleteComment } from '../services/api';

const PostedComment = ({ comment, refreshComments }) => {
  const { user_id, username, body, created_at, id } = comment;
  const loggedInUserId = localStorage.getItem('userId');
  const isOwner = loggedInUserId && String(user_id) === loggedInUserId;
  const profileSlug = username || user_id;

  const handleDelete = async () => {
    try {
      await deleteComment(id);
      if (refreshComments) refreshComments();
    } catch (error) {
      console.error('Error deleting comment:', error);
      alert('Error deleting comment.');
    }
  };

  return (
    <div style={{ marginBottom: '10px', padding: '10px', borderBottom: '1px solid #ddd', position: 'relative' }}>
      {/* Username and Timestamp */}
      <div>
        <Link 
          to={`/users/${profileSlug}`} 
          style={{ fontSize: '18px', fontWeight: 'bold', textDecoration: 'none', color: '#3b5998' }}
        >
          {username}
        </Link>
        <span style={{ marginLeft: '10px', color: '#555', fontSize: '12px' }}>
          {new Date(created_at).toLocaleString()}
        </span>
        {isOwner && (
          <button 
            onClick={handleDelete} 
            style={{
              position: 'absolute',
              right: '10px',
              top: '10px',
              background: 'transparent',
              border: 'none',
              color: 'red',
              cursor: 'pointer'
            }}
          >
            Delete
          </button>
        )}
      </div>
      {/* Comment Body */}
      <div style={{ marginTop: '5px', fontSize: '14px', color: '#333' }}>
        {body}
      </div>
    </div>
  );
};

export default PostedComment;
