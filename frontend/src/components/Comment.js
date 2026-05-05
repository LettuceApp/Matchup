import React, { useState } from 'react';
import { Link } from 'react-router-dom';
import { FiFlag } from 'react-icons/fi';
import Button from './Button';
import ReportModal from './ReportModal';
import { deleteComment } from '../services/api';
import '../styles/Comment.css';

/*
 * Comment — shared renderer for matchup + bracket comments.
 *
 * `subjectType` ("comment" | "bracket_comment") controls what kind of
 * entity a Report opens against. Defaults to the matchup-side value to
 * preserve older callsites that didn't pass the prop; BracketPage
 * passes "bracket_comment" explicitly.
 *
 * A Report affordance is available to logged-in non-owners. Owners
 * don't see Report on their own comment (can't meaningfully report
 * yourself); anonymous viewers don't either (no rate-limit subject).
 */
const Comment = ({
  comment,
  refreshComments,
  onDelete,
  subjectType = 'comment',
}) => {
  const [reportOpen, setReportOpen] = useState(false);

  if (!comment) return null;

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

  // Reports key on the comment's public_id. Older server payloads that
  // only carry the numeric id can't be reported; fall back to null and
  // hide the button rather than firing a CodeInvalidArgument.
  const reportableId = comment.public_id || comment.publicId || null;

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

      <div className="comment-card__actions">
        {isOwner && (
          <Button onClick={handleDelete} className="comment-card__delete">
            Delete
          </Button>
        )}
        {!isOwner && userId && reportableId && (
          <button
            type="button"
            className="comment-card__report"
            aria-label="Report this comment"
            onClick={() => setReportOpen(true)}
          >
            <FiFlag aria-hidden="true" /> Report
          </button>
        )}
      </div>

      {reportOpen && reportableId && (
        <ReportModal
          subjectType={subjectType}
          subjectId={reportableId}
          onClose={() => setReportOpen(false)}
        />
      )}
    </article>
  );
};

export default Comment;
