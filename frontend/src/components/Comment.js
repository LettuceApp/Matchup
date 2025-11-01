import React from 'react';
import { Link } from 'react-router-dom';
import Button from './Button';
import { deleteComment } from '../services/api';
import '../styles/Comment.css';

const Comment = ({ comment, refreshComments }) => {
  const storedUserId = localStorage.getItem('userId');
  const userId = storedUserId ? parseInt(storedUserId, 10) : null;
  const isOwner = comment.user_id === userId;

  const handleDelete = async () => {
    try {
      await deleteComment(comment.id);
      if (refreshComments) {
        refreshComments();
      }
    } catch (error) {
      console.error('Error deleting comment:', error);
      alert('Failed to delete comment.');
    }
  };

  return (
    <article className="comment-card">
      <header className="comment-card__header">
        <Link
          to={`/users/${comment.user_id}/profile`}
          className="comment-card__author"
        >
          {comment.username}
        </Link>
        <span className="comment-card__timestamp">
          {new Date(comment.created_at).toLocaleString()}
        </span>
      </header>

      <p className="comment-card__body">{comment.body}</p>

      {isOwner && (
        <div className="comment-card__actions">
          <Button onClick={handleDelete} className="comment-card__delete">
            Delete
          </Button>
        </div>
      )}
    </article>
  );
};

export default Comment;
