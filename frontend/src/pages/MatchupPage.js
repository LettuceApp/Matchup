import React, { useState, useEffect, useCallback } from 'react';
import { useNavigate, useParams, Link } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import MatchupItem from '../components/MatchupItem';
import Comment from '../components/Comment';
import Button from '../components/Button';
import {
  getUserMatchup,
  likeMatchup,
  unlikeMatchup,
  getUserLikes,
  createComment,
  getComments,
  deleteMatchup
} from '../services/api';
import '../styles/MatchupPage.css';

const MatchupPage = () => {
  const { uid, id } = useParams();
  const navigate = useNavigate();
  const userId = localStorage.getItem('userId');
  const [matchup, setMatchup] = useState(null);
  const [likesCount, setLikesCount] = useState(0);
  const [isLiked, setIsLiked] = useState(false);
  const [comments, setComments] = useState([]);
  const [newComment, setNewComment] = useState('');
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [commentError, setCommentError] = useState(null);
  const [commentPending, setCommentPending] = useState(false);
  const [likePending, setLikePending] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);

  const loadMatchup = useCallback(
    async (withSpinner = false) => {
      if (withSpinner) {
        setIsLoading(true);
      }

      try {
        const response = await getUserMatchup(uid, id);
        const matchupData = response.data.response ?? response.data;
        setMatchup(matchupData);
        setLikesCount(matchupData?.likes_count ?? 0);

        if (userId) {
          const userLikesResponse = await getUserLikes(userId);
          const userLikes = userLikesResponse.data.response ?? userLikesResponse.data;
          const likedMatchup = (userLikes || []).some(
            (like) => like.matchup_id === parseInt(id, 10)
          );
          setIsLiked(likedMatchup);
        } else {
          setIsLiked(false);
        }

        const commentsResponse = await getComments(id);
        const commentsData = commentsResponse.data.response ?? commentsResponse.data;
        setComments(Array.isArray(commentsData) ? commentsData : []);
        setError(null);
      } catch (err) {
        console.error('Failed to fetch matchup:', err);
        setError('We had trouble loading this matchup. Please try again.');
      } finally {
        if (withSpinner) {
          setIsLoading(false);
        }
      }
    },
    [uid, id, userId]
  );

  useEffect(() => {
    loadMatchup(true);
  }, [loadMatchup]);

  const refreshMatchup = useCallback(() => {
    loadMatchup(false);
  }, [loadMatchup]);

  const handleLikeToggle = async () => {
    if (likePending) {
      return;
    }

    try {
      setLikePending(true);
      if (isLiked) {
        await unlikeMatchup(id);
        setLikesCount((prev) => Math.max(0, prev - 1));
      } else {
        await likeMatchup(id);
        setLikesCount((prev) => prev + 1);
      }
      setIsLiked((prev) => !prev);
    } catch (err) {
      console.error('Failed to toggle like on matchup:', err);
      setError('We could not update your like just now. Please try again.');
    } finally {
      setLikePending(false);
    }
  };

  const handleCommentSubmit = async (e) => {
    e.preventDefault();
    const trimmedComment = newComment.trim();
    if (!trimmedComment) {
      return;
    }

    try {
      setCommentPending(true);
      setCommentError(null);
      await createComment(id, { Body: trimmedComment });
      setNewComment('');
      refreshMatchup();
    } catch (err) {
      console.error('Failed to add comment:', err);
      setCommentError('We could not post that comment. Please try again.');
    } finally {
      setCommentPending(false);
    }
  };

  const handleDelete = async () => {
    if (isDeleting) {
      return;
    }

    try {
      setIsDeleting(true);
      await deleteMatchup(id);
      navigate('/home', { replace: true });
    } catch (err) {
      console.error('Failed to delete matchup:', err);
      setError('We could not delete this matchup. Please try again.');
    } finally {
      setIsDeleting(false);
    }
  };

  if (!matchup && isLoading) {
    return (
      <div className="matchup-page">
        <NavigationBar />
        <main className="matchup-content matchup-content--center">
          <div className="matchup-status-card">Loading matchup...</div>
        </main>
      </div>
    );
  }

  if (!matchup && error) {
    return (
      <div className="matchup-page">
        <NavigationBar />
        <main className="matchup-content matchup-content--center">
          <div className="matchup-status-card matchup-status-card--error">{error}</div>
          <Button onClick={() => loadMatchup(true)} className="matchup-secondary-button">
            Try again
          </Button>
        </main>
      </div>
    );
  }

  if (!matchup) {
    return null;
  }

  const isOwner = userId ? matchup.author_id === parseInt(userId, 10) : false;

  const authorName =
    matchup?.author?.username ||
    matchup?.author?.name ||
    matchup?.author_username ||
    matchup?.author_name ||
    matchup?.Author?.username ||
    matchup?.Author?.name ||
    null;

  const formattedCreatedAt = matchup?.created_at
    ? new Date(matchup.created_at).toLocaleString()
    : null;

  return (
    <div className="matchup-page">
      <NavigationBar />
      <main className="matchup-content">
        {error && (
          <div className="matchup-status-card matchup-status-card--warning">
            {error}
          </div>
        )}

        <section className="matchup-hero">
          <div className="matchup-hero-text">
            <p className="matchup-overline">
              Matchup Detail
              {authorName && (
                <>
                  {' · '}
                  <Link
                    to={`/users/${matchup.author_id}/profile`}
                    className="matchup-author-link"
                  >
                    {authorName}
                  </Link>
                </>
              )}
            </p>
            <h1>{matchup.title}</h1>
            <p className="matchup-description">{matchup.content}</p>
            <div className="matchup-meta">
              {formattedCreatedAt && <span>Published {formattedCreatedAt}</span>}
            </div>
            <div className="matchup-actions">
              <Button
                onClick={handleLikeToggle}
                className={`matchup-like-button${isLiked ? ' is-liked' : ''}`}
                disabled={likePending}
              >
                {isLiked ? 'Unlike matchup' : 'Like matchup'}
              </Button>
              <div className="matchup-like-indicator">
                <span className="matchup-like-count">{likesCount}</span>
                <span>cheers</span>
              </div>
              {isOwner && (
                <Button
                  onClick={handleDelete}
                  className="matchup-danger-button"
                  disabled={isDeleting}
                >
                  {isDeleting ? 'Deleting...' : 'Delete matchup'}
                </Button>
              )}
            </div>
          </div>

          <div className="matchup-hero-stats" aria-hidden="true">
            <div className="matchup-stat-card">
              <span className="matchup-stat-label">Contenders</span>
              <span className="matchup-stat-value">{matchup.items?.length ?? 0}</span>
            </div>
            <div className="matchup-stat-card">
              <span className="matchup-stat-label">Likes</span>
              <span className="matchup-stat-value">{likesCount}</span>
            </div>
          </div>
        </section>

        <section className="matchup-section">
          <header className="matchup-section-header">
            <div>
              <h2>Contenders & Votes</h2>
              <p>Tap a contender to edit its name if you own this matchup.</p>
            </div>
          </header>
          <div className="matchup-items">
            {matchup.items?.length ? (
              matchup.items.map((item) => (
                <MatchupItem
                  key={item.id}
                  item={item}
                  isOwner={isOwner}
                  refreshItems={refreshMatchup}
                />
              ))
            ) : (
              <div className="matchup-status-card matchup-status-card--muted">
                No contenders yet.
              </div>
            )}
          </div>
        </section>

        <section className="matchup-section">
          <header className="matchup-section-header">
            <div>
              <h2>Comments</h2>
              <p>Join the conversation and share your perspective.</p>
            </div>
          </header>

          {comments.length > 0 ? (
            <div className="matchup-comments-list">
              {comments.map((comment) => (
                <Comment key={comment.id} comment={comment} refreshComments={refreshMatchup} />
              ))}
            </div>
          ) : (
            <div className="matchup-status-card matchup-status-card--muted">
              Be the first to comment on this matchup.
            </div>
          )}

          <form onSubmit={handleCommentSubmit} className="matchup-comment-form">
            <label htmlFor="comment-body" className="matchup-form-label">
              Add a comment
            </label>
            <textarea
              id="comment-body"
              value={newComment}
              onChange={(e) => setNewComment(e.target.value)}
              placeholder="Share your take..."
              className="matchup-textarea"
              rows={4}
              required
            />
            {commentError && <p className="matchup-inline-error">{commentError}</p>}
            <div className="matchup-form-actions">
              <Button
                type="submit"
                className="matchup-primary-button"
                disabled={commentPending}
              >
                {commentPending ? 'Posting...' : 'Post comment'}
              </Button>
            </div>
          </form>
        </section>
      </main>
    </div>
  );
};

export default MatchupPage;
