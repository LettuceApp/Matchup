import React from 'react';
import { Link } from 'react-router-dom';
import Button from './Button';
import { deleteComment } from '../services/api';
import '../styles/Comment.css';

const Comment = ({ comment, refreshComments, onDelete }) => {
  const storedUserId = localStorage.getItem('userId');
  const userId = storedUserId || null;
  const isOwner = userId && String(comment.user_id) === userId;
  const profileSlug = comment.username || comment.user_id;
  const decodeHtml = (value) => {
    if (!value || typeof value !== 'string') return value;
    if (typeof document === 'undefined') return value;
    const textarea = document.createElement('textarea');
    textarea.innerHTML = value;
    return textarea.value;
  };

  const handleDelete = async () => {
    try {
      const deleteFn = onDelete || deleteComment;
      await deleteFn(comment.id);
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
          to={`/users/${profileSlug}`}
          className="comment-card__author"
        >
          {comment.username}
        </Link>
        <span className="comment-card__timestamp">
          {new Date(comment.created_at).toLocaleString()}
        </span>
      </header>

      <p className="comment-card__body">{decodeHtml(comment.body)}</p>

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
