import React, { useState, useEffect, useCallback, useRef } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import MatchupItem from "../components/MatchupItem";
import Comment from "../components/Comment";
import Button from "../components/Button";
import ConfirmModal from "../components/ConfirmModal";
import SkeletonCard from "../components/SkeletonCard";
import {
  getMatchup,
  getUserMatchup,
  likeMatchup,
  unlikeMatchup,
  getUserLikes,
  createComment,
  getComments,
  deleteMatchup,
  getBracket,
  activateMatchup,
  overrideMatchupWinner,
  completeMatchup,
  getCurrentUser,
  updateBracket,
} from "../services/api";
import "../styles/MatchupPage.css";
import useCountdown from "../hooks/useCountdown";

const MatchupPage = () => {
  const { uid, id } = useParams();
  const navigate = useNavigate();
  const userId = localStorage.getItem("userId");
  const viewerId = userId || null;

  const [currentUser, setCurrentUser] = useState(null);
  const [matchup, setMatchup] = useState(null);
  const [bracket, setBracket] = useState(null);
  const [likesCount, setLikesCount] = useState(0);
  const [isLiked, setIsLiked] = useState(false);
  const [comments, setComments] = useState([]);
  const [newComment, setNewComment] = useState("");

  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [commentError, setCommentError] = useState(null);
  const [commentPending, setCommentPending] = useState(false);
  const [likePending, setLikePending] = useState(false);
  const [readyPending, setReadyPending] = useState(false);
  const [activatePending, setActivatePending] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [confirmModal, setConfirmModal] = useState(null); // { message, confirmLabel, danger, onConfirm }
  const [votedItemId, setVotedItemId] = useState(null);

  /* ------------------------------------------------------------------ */
  /* DATA LOADING */
  /* ------------------------------------------------------------------ */

  const refreshMatchup = useCallback(async () => {
    // ✅ Prefer "get by ID" so bracket matchups never 404 due to uid mismatch
    let matchupData;
    try {
      const res = await getMatchup(id);
      matchupData = res.data?.matchup ?? res.data?.response ?? res.data;
    } catch (e) {
      if (e?.response?.status === 403) {
        throw e;
      }
      // fallback for older setups
      const res = await getUserMatchup(uid, id);
      matchupData = res.data?.matchup ?? res.data?.response ?? res.data;
    }

    setMatchup(matchupData);

    const commentsRes = await getComments(id);
    const rawComments = commentsRes.data?.comments ?? commentsRes.data?.response ?? commentsRes.data;
    setComments(Array.isArray(rawComments) ? rawComments : []);

    // ✅ Likes should represent the logged-in viewer, not the UID in the URL
    if (viewerId) {
      const likesRes = await getUserLikes(viewerId);
      const likes = likesRes.data?.likes ?? likesRes.data?.response ?? likesRes.data ?? [];
      const liked = Array.isArray(likes)
        ? likes.some((l) => String(l.matchup_id) === String(id))
        : false;
      setIsLiked(liked);
    } else {
      setIsLiked(false);
    }

    setLikesCount(Number(matchupData.likes_count ?? matchupData.likesCount ?? 0));

    if (matchupData.bracket_id) {
      const b = await getBracket(matchupData.bracket_id);
      setBracket(b.data?.bracket ?? b.data?.response ?? b.data);
    } else {
      setBracket(null);
    }
  }, [uid, id, viewerId]);

  const loadMatchup = useCallback(async (initial = false) => {
    try {
      if (initial) setIsLoading(true);
      await refreshMatchup();
    } catch (err) {
      console.error(err);
      setError(
        err?.response?.status === 403
          ? "Only followers can view this matchup."
          : "We couldn't load this matchup right now."
      );
    } finally {
      if (initial) setIsLoading(false);
    }
  }, [refreshMatchup]);

  useEffect(() => {
    loadMatchup(true);
  }, [loadMatchup]);

  useEffect(() => {
    if (matchup?.title) {
      document.title = `${matchup.title} | Matchup Hub`;
      return () => { document.title = 'Matchup Hub'; };
    }
  }, [matchup?.title]);

  useEffect(() => {
    async function loadMe() {
      try {
        const res = await getCurrentUser();
        setCurrentUser(res.data?.user ?? res.data?.response ?? res.data);
      } catch (err) {
        console.warn("Unable to load current user", err);
      }
    }
    loadMe();
  }, []);

  /* ------------------------------------------------------------------ */
  /* TIMER + EXPIRATION */
  /* ------------------------------------------------------------------ */

  const matchupRound = matchup?.round ?? matchup?.Round ?? null;
  const isBracketMatchup = Boolean(matchup?.bracket_id);

  const bracketAdvanceMode =
    bracket?.advance_mode ?? bracket?.advanceMode ?? "manual";
  const bracketRoundEndsAt =
    isBracketMatchup &&
    bracketAdvanceMode === "timer" &&
    Number(matchupRound) === Number(bracket?.current_round ?? bracket?.currentRound)
      ? bracket?.round_ends_at ?? bracket?.roundEndsAt ?? null
      : null;

  const matchupEndsAt =
    matchup?.end_time ?? matchup?.EndTime ?? bracketRoundEndsAt;
  const countdown = useCountdown(matchupEndsAt);
  const hasRefreshedAfterExpiry = useRef(false);

  const matchupStatus = matchup?.status ?? matchup?.Status ?? "active";
  const isOpenStatus = matchupStatus === "published" || matchupStatus === "active";

  const matchupExpired = countdown.isExpired || matchupStatus === "completed";

  /* ------------------------------------------------------------------ */
  /* PERMISSIONS + LOCKS */
  /* ------------------------------------------------------------------ */

  const isOwner = currentUser?.id && matchup?.author_id && currentUser.id === matchup.author_id;

  const isAdmin = currentUser?.is_admin === true;

  const canManageBracket = isBracketMatchup && (isOwner || isAdmin);
  const showBracketActivate =
    canManageBracket && bracket?.status === "draft";

  const isActiveBracketRound =
    isBracketMatchup &&
    bracket?.status === "active" &&
    Number(matchupRound) === Number(bracket?.current_round ?? bracket?.currentRound);

  const isVotingLocked =
    !isOwner && (
      matchupExpired ||
      (!isBracketMatchup && !isOpenStatus) ||
      (isBracketMatchup &&
        (!bracket ||
          bracket.status !== "active" ||
          Number(matchupRound) !==
            Number(bracket?.current_round ?? bracket?.currentRound))));

  const interactionLocked =
    matchupExpired ||
    (!isBracketMatchup && !isOpenStatus) ||
    (isBracketMatchup &&
      (!bracket ||
        bracket.status !== "active" ||
        Number(matchupRound) !==
          Number(bracket?.current_round ?? bracket?.currentRound)));
  const isReady = matchupStatus === "completed";
  const canReadyUp = (isOwner || isAdmin) && (isOpenStatus || isReady);
  const canLike = Boolean(viewerId) && matchupStatus !== "draft";
  const canComment = Boolean(viewerId) && matchupStatus !== "draft";
  const winnerMenuRef = useRef(null);
  const canActivate =
    !isBracketMatchup &&
    isOwner &&
    matchupStatus === "draft" &&
    !activatePending;

  /* ------------------------------------------------------------------ */
  /* DERIVED MATCHUP STATE */
  /* ------------------------------------------------------------------ */

  const items = matchup?.items ?? matchup?.Items ?? [];
  const totalVotes = items.reduce(
    (sum, item) => sum + Number(item?.votes ?? item?.Votes ?? 0),
    0
  );

  const winnerItemIdRaw =
    matchup?.winner_item_id ?? matchup?.winnerItemId ?? matchup?.WinnerItemID ?? null;

  const winnerItemId =
    winnerItemIdRaw !== null && winnerItemIdRaw !== undefined
      ? winnerItemIdRaw
      : null;

  const displayWinnerId = isReady ? winnerItemId : null;

  useEffect(() => {
    hasRefreshedAfterExpiry.current = false;
  }, [matchup?.id, matchupEndsAt]);

  useEffect(() => {
    if (isBracketMatchup) return;
    if (!countdown.isExpired) return;
    if (matchupStatus === "completed" && winnerItemId !== null) return;
    if (hasRefreshedAfterExpiry.current) return;

    hasRefreshedAfterExpiry.current = true;
    refreshMatchup();
  }, [
    isBracketMatchup,
    countdown.isExpired,
    matchupStatus,
    winnerItemId,
    refreshMatchup,
  ]);

  const canOverrideWinner =
    isBracketMatchup &&
    (isOwner || isAdmin) &&
    bracket?.status === "active" &&
    isActiveBracketRound;

  let highestVotes = Number.NEGATIVE_INFINITY;
  let topCount = 0;

  items.forEach((item) => {
    const v = Number(item.votes ?? item.Votes ?? 0);
    if (v > highestVotes) {
      highestVotes = v;
      topCount = 1;
    } else if (v === highestVotes) {
      topCount += 1;
    }
  });

  const leadingVotes = topCount === 1 ? highestVotes : null;

  const votesAreTied = items.length > 0 && topCount > 1;

  const requiresManualWinner = !isReady && winnerItemId === null && votesAreTied;

  const readyEnabled = canReadyUp && (!isReady ? !requiresManualWinner : true);

  // ✅ FIX: this is what your build complained was missing
  const isTieAfterExpiryInActiveRound =
    countdown.isExpired &&
    isActiveBracketRound &&
    votesAreTied &&
    winnerItemId === null &&
    matchupStatus !== "completed";

  /* ------------------------------------------------------------------ */
  /* ACTIONS */
  /* ------------------------------------------------------------------ */

  const handleDelete = async () => {
    if (!matchup) return;

    if (matchup.bracket_id) {
      alert("Bracket matchups can't be deleted individually.");
      return;
    }

    try {
      setIsDeleting(true);
      await deleteMatchup(matchup.id);
      const profileSlug = matchup?.author?.username || uid;
      navigate(`/users/${profileSlug}`);
    } catch (err) {
      console.error(err);
      setError("Unable to delete matchup.");
    } finally {
      setIsDeleting(false);
    }
  };

  

  const handleLikeToggle = async () => {
    if (!matchup || likePending || !canLike) return;

    try {
      setLikePending(true);
      if (isLiked) {
        await unlikeMatchup(matchup.id);
        setIsLiked(false);
        setLikesCount((c) => Math.max(0, c - 1));
      } else {
        await likeMatchup(matchup.id);
        setIsLiked(true);
        setLikesCount((c) => c + 1);
      }
    } catch (err) {
      console.error(err);
      setError(err?.response?.data?.message || err?.response?.data?.error || "Unable to update like.");
    } finally {
      setLikePending(false);
    }
  };

  const handleCommentSubmit = async (e) => {
    e.preventDefault();
    if (!newComment.trim() || commentPending || !canComment) return;

    try {
      setCommentPending(true);
      setCommentError(null);
      await createComment(id, { body: newComment.trim() });
      setNewComment("");
      await refreshMatchup();
    } catch (err) {
      console.error(err);
      setCommentError("Unable to post comment.");
    } finally {
      setCommentPending(false);
    }
  };

  const handleOverrideWinner = (winnerId) => {
    if (!canOverrideWinner) return;
    const msg = isTieAfterExpiryInActiveRound
      ? "Select this winner? You can still change it until the round advances."
      : "Override votes and select this winner?";
    setConfirmModal({
      message: msg,
      confirmLabel: 'Confirm',
      danger: false,
      onConfirm: async () => {
        try {
          await overrideMatchupWinner(matchup.id, winnerId);
          await refreshMatchup();
        } catch (err) {
          console.error(err);
          setError("Failed to select winner.");
        }
      },
    });
  };

  const handleReadyUp = () => {
    if (!readyEnabled || readyPending) return;
    const promptMessage = isReady ? "Undo ready?" : "Ready up and lock the winner?";
    setConfirmModal({
      message: promptMessage,
      confirmLabel: isReady ? 'Undo' : 'Ready up',
      danger: false,
      onConfirm: async () => {
        try {
          setReadyPending(true);
          await completeMatchup(matchup.id);
          await refreshMatchup();
        } catch (err) {
          console.error(err);
          setError("Could not update matchup readiness.");
        } finally {
          setReadyPending(false);
        }
      },
    });
  };

  const handleActivateMatchup = () => {
    if (!canActivate) return;
    setConfirmModal({
      message: 'Activate this matchup?',
      confirmLabel: 'Activate',
      danger: false,
      onConfirm: async () => {
        try {
          setActivatePending(true);
          await activateMatchup(matchup.id);
          await refreshMatchup();
        } catch (err) {
          console.error(err);
          setError("Unable to activate matchup.");
        } finally {
          setActivatePending(false);
        }
      },
    });
  };

  /* ------------------------------------------------------------------ */
  /* RENDER */
  /* ------------------------------------------------------------------ */

  if (isLoading) {
    return (
      <div className="matchup-page">
        <NavigationBar />
        <main className="matchup-content">
          <div className="matchup-skeleton-grid">
            <SkeletonCard lines={3} />
            <SkeletonCard lines={2} />
          </div>
        </main>
      </div>
    );
  }

  if (!matchup) {
    return (
      <div className="matchup-page">
        <NavigationBar />
        <main className="matchup-content">
          <div className="matchup-status-card matchup-status-card--error">
            Matchup not found.
          </div>
        </main>
      </div>
    );
  }

  const authorName =
    matchup.author?.username ??
    matchup.author_username ??
    matchup.author?.Username ??
    "Unknown";

  return (
    <div className="matchup-page">
      <NavigationBar />
      <main className="matchup-content">
        {error && (
          <div className="matchup-status-card matchup-status-card--error">
            {error}
          </div>
        )}

        <section className="matchup-section">
          {matchup.image_url && (
            <div className="matchup-cover-image">
              {/* Cover hero is above the fold — keep eager-loaded but async-decoded */}
              <img src={matchup.image_url} alt={matchup.title} decoding="async" />
            </div>
          )}

          {Array.isArray(matchup.tags) && matchup.tags.length > 0 && (
            <div className="matchup-tags">
              {matchup.tags.map((tag) => (
                <span key={tag} className="matchup-tag">{tag}</span>
              ))}
            </div>
          )}

          {isBracketMatchup && bracket && (
            <div className="matchup-bracket-crumb">
              <Link to={`/brackets/${matchup.bracket_id}`} className="matchup-bracket-crumb__link">
                ← {bracket.title || "Back to bracket"}
              </Link>
            </div>
          )}

          <div className="matchup-meta">
            <div className="matchup-meta__left">
              <h2>
                Matchup{" "}
                {!isBracketMatchup && (
                  <>
                    ·{" "}
                    <Link
                      to={`/users/${matchup?.author?.username || matchup.author_id}`}
                      className="matchup-author-link"
                    >
                      {authorName}
                    </Link>
                  </>
                )}
              </h2>
              <p className="matchup-subtitle">🗳 Tap a contender to cast your vote.</p>
              <div className="matchup-status-row">
                <div className="matchup-status-pill">
                  Status: <strong>{matchupStatus || "unknown"}</strong>
                </div>
                {matchupEndsAt && !matchupExpired && (
                  <div className="matchup-status-pill matchup-status-pill--timer">
                    ⏳ <strong>{countdown.formatted}</strong>
                  </div>
                )}
                {matchupExpired && (
                  <div className="matchup-status-pill matchup-status-pill--locked">
                    ⛔ Voting closed
                  </div>
                )}
              </div>
              {isTieAfterExpiryInActiveRound && (
                <div className="matchup-status-banner matchup-status-banner--warning">
                  ⚠️ Voting ended in a tie. The owner must choose a winner before the round can advance.
                </div>
              )}
            </div>

            <div className="matchup-meta__right">
              <div className="matchup-actions">
                {viewerId ? (
                  <button
                    type="button"
                    className={`matchup-like-button ${isLiked ? "is-liked" : ""}`}
                    onClick={handleLikeToggle}
                    disabled={!canLike || likePending}
                  >
                    {likePending ? "Updating…" : isLiked ? "❤️ Liked" : "♡ Like"}
                  </button>
                ) : null}
                <div className="matchup-like-indicator">
                  <span className="matchup-like-count">{likesCount}</span>
                  <span>Likes</span>
                </div>
              </div>

              {canReadyUp && (
                <div className="matchup-readyup-group">
                  <Button
                    onClick={handleReadyUp}
                    disabled={!readyEnabled || readyPending}
                    title="Mark this matchup as complete and reveal the winner"
                    className={`matchup-readyup-button ${isReady ? "matchup-readyup-button--armed" : ""}`}
                  >
                    {readyPending ? (isReady ? "Unlocking…" : "Locking…") : isReady ? "Undo Ready" : "Ready up"}
                  </Button>
                  <span className="matchup-readyup-hint">Closes voting &amp; reveals winner</span>
                </div>
              )}

              {canActivate && (
                <Button
                  onClick={handleActivateMatchup}
                  disabled={activatePending}
                  className="matchup-readyup-button"
                >
                  {activatePending ? "Activating…" : "Activate matchup"}
                </Button>
              )}

              {isVotingLocked && (
                <span className="matchup-badge matchup-badge--locked">
                  Voting locked
                </span>
              )}

              {canOverrideWinner && items.length > 0 && (
                <details ref={winnerMenuRef} className="matchup-winner-menu">
                  <summary className="matchup-winner-summary" aria-label="Winner options">
                    ⋯
                  </summary>
                  <div className="matchup-winner-panel">
                    <p className="matchup-winner-title">Select winner</p>
                    {items.map((item) => {
                      const itemLabel = item.item ?? item.name ?? "Contender";
                      const isSelected = winnerItemId === item.id;
                      return (
                        <button
                          key={`winner-${item.id}`}
                          type="button"
                          className={`matchup-winner-option ${isSelected ? "is-selected" : ""}`}
                          onClick={() => {
                            handleOverrideWinner(item.id);
                            if (winnerMenuRef.current) {
                              winnerMenuRef.current.removeAttribute("open");
                            }
                          }}
                        >
                          {itemLabel}
                        </button>
                      );
                    })}
                  </div>
                </details>
              )}
            </div>
          </div>

          <div className="matchup-items">
            {items.map((item) => (
              <MatchupItem
                key={item.id}
                item={item}
                totalVotes={totalVotes}
                showVoteBar
                isWinner={displayWinnerId === item.id}
                isLeading={
                  displayWinnerId === null &&
                  leadingVotes !== null &&
                  Number(item?.votes ?? item?.Votes ?? 0) === Number(leadingVotes)
                }
                hasWinner={displayWinnerId !== null}
                allowEdit={
                  isOwner &&
                  (!isBracketMatchup || bracket?.status === "draft")
                }
                isVotingLocked={isVotingLocked}
                isBracketMatchup={isBracketMatchup}
                canOverrideWinner={canOverrideWinner}
                disabled={isVotingLocked}
                onOverrideWinner={() => handleOverrideWinner(item.id)}
                onVote={() => { setVotedItemId(item.id); return refreshMatchup(); }}
                isOwner={isOwner}
                isVoted={votedItemId === item.id}
              />
            ))}
          </div>
        </section>

        <section className="matchup-section matchup-section--comments">
          <header className="matchup-section-header matchup-section-header--comments">
            <div>
              <h2>Comments</h2>
              <p>Join the debate.</p>
            </div>
          </header>

          <div className="matchup-comments">
            {!Array.isArray(comments) || comments.length === 0 ? (
              <div className="matchup-empty-comments">
                <span className="matchup-empty-comments__icon">💬</span>
                <p>Be the first to share your take!</p>
                <span className="matchup-empty-comments__arrow">↓</span>
              </div>
            ) : (
              comments.filter(Boolean).map((comment) => (
                <Comment key={comment.id} comment={comment} />
              ))
            )}
          </div>

          {viewerId ? (
            <form className="matchup-comment-form" onSubmit={handleCommentSubmit}>
              <label htmlFor="newComment" className="matchup-form-label">
                Add a comment
              </label>
              <textarea
                id="newComment"
                value={newComment}
                onChange={(e) => {
                  setNewComment(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = e.target.scrollHeight + 'px';
                }}
                rows={1}
                disabled={commentPending || !canComment}
                className="matchup-textarea"
                placeholder="Share your take…"
              />
              {commentError && <p className="matchup-inline-error">{commentError}</p>}
              <div className="matchup-form-actions">
                <button
                  type="submit"
                  disabled={commentPending || !canComment}
                  className="matchup-primary-button"
                >
                  {commentPending ? "Posting…" : "Post comment"}
                </button>
              </div>
            </form>
          ) : null}
        </section>

        {isOwner && !isBracketMatchup && (
          <div className="matchup-danger-zone">
            <Button
              onClick={() => setDeleteModalOpen(true)}
              disabled={isDeleting}
              className="matchup-danger-button"
            >
              {isDeleting ? "Deleting…" : "Delete matchup"}
            </Button>
          </div>
        )}

        {deleteModalOpen && (
          <div className="edit-profile-overlay" onClick={() => setDeleteModalOpen(false)}>
            <div className="edit-profile-modal" onClick={(e) => e.stopPropagation()}>
              <h2 className="edit-profile-title">Delete matchup?</h2>
              <p style={{ color: 'rgba(226,232,240,0.7)', fontSize: '0.9rem', margin: 0 }}>
                This can't be undone.
              </p>
              <div className="edit-profile-actions">
                <Button className="profile-secondary-button" onClick={() => setDeleteModalOpen(false)} disabled={isDeleting}>
                  Cancel
                </Button>
                <Button className="matchup-danger-button" onClick={() => { setDeleteModalOpen(false); handleDelete(); }} disabled={isDeleting}>
                  {isDeleting ? 'Deleting…' : 'Delete'}
                </Button>
              </div>
            </div>
          </div>
        )}
      </main>

      {confirmModal && (
        <ConfirmModal
          message={confirmModal.message}
          confirmLabel={confirmModal.confirmLabel}
          danger={confirmModal.danger}
          onConfirm={() => { confirmModal.onConfirm(); setConfirmModal(null); }}
          onCancel={() => setConfirmModal(null)}
        />
      )}
    </div>
  );
};

export default MatchupPage;
