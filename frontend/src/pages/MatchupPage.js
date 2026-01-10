import React, { useState, useEffect, useCallback, useRef } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import MatchupItem from "../components/MatchupItem";
import Comment from "../components/Comment";
import Button from "../components/Button";
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
  overrideMatchupWinner,
  completeMatchup,
  getCurrentUser,
  activateMatchup,
} from "../services/api";
import "../styles/MatchupPage.css";
import useCountdown from "../hooks/useCountdown";

const MatchupPage = () => {
  const { uid, id } = useParams();
  const navigate = useNavigate();
  const userId = localStorage.getItem("userId");
  const viewerId = Number(userId || 0);

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
  const [isDeleting, setIsDeleting] = useState(false);

  /* ------------------------------------------------------------------ */
  /* DATA LOADING */
  /* ------------------------------------------------------------------ */

  const refreshMatchup = useCallback(async () => {
    // ✅ Prefer "get by ID" so bracket matchups never 404 due to uid mismatch
    let matchupData;
    try {
      const res = await getMatchup(id);
      matchupData = res.data?.response ?? res.data;
    } catch (e) {
      // fallback for older setups
      const res = await getUserMatchup(uid, id);
      matchupData = res.data?.response ?? res.data;
    }

    setMatchup(matchupData);

    const commentsRes = await getComments(id);
    setComments(commentsRes.data.response ?? commentsRes.data ?? []);

    // ✅ Likes should represent the logged-in viewer, not the UID in the URL
    if (viewerId) {
      const likesRes = await getUserLikes(viewerId);
      const likes = likesRes.data.response ?? likesRes.data ?? [];
      const liked = Array.isArray(likes)
        ? likes.some((l) => l.matchup_id === Number(id))
        : false;
      setIsLiked(liked);
    } else {
      setIsLiked(false);
    }

    setLikesCount(matchupData.likes_count ?? matchupData.likesCount ?? 0);

    if (matchupData.bracket_id) {
      const b = await getBracket(matchupData.bracket_id);
      setBracket(b.data.response ?? b.data);
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
      setError("We couldn't load this matchup right now.");
    } finally {
      if (initial) setIsLoading(false);
    }
  }, [refreshMatchup]);

  useEffect(() => {
    loadMatchup(true);
  }, [loadMatchup]);

  useEffect(() => {
    async function loadMe() {
      try {
        const res = await getCurrentUser();
        setCurrentUser(res.data.response ?? res.data);
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

  const isOwner = viewerId && matchup?.author_id === Number(viewerId);

  const isAdmin = (currentUser?.role ?? currentUser?.Role) === "admin";

  const showActivateButton =
    !isBracketMatchup &&
    isOwner &&
    matchupStatus === "draft";

  const isActiveBracketRound =
    isBracketMatchup &&
    bracket?.status === "active" &&
    Number(matchupRound) === Number(bracket?.current_round ?? bracket?.currentRound);

  const isVotingLocked =
    matchupExpired ||
    (!isBracketMatchup && !isOpenStatus) ||
    (isBracketMatchup &&
      (!bracket ||
        bracket.status !== "active" ||
        Number(matchupRound) !==
          Number(bracket?.current_round ?? bracket?.currentRound)));

  const interactionLocked =
    matchupExpired ||
    (!isBracketMatchup && !isOpenStatus) ||
    (isBracketMatchup &&
      (!bracket ||
        bracket.status !== "active" ||
        Number(matchupRound) !==
          Number(bracket?.current_round ?? bracket?.currentRound)));
  const canReadyUp = isOwner || isAdmin;
  const canLike = Boolean(viewerId) && !interactionLocked;

  /* ------------------------------------------------------------------ */
  /* DERIVED MATCHUP STATE */
  /* ------------------------------------------------------------------ */

  const items = matchup?.items ?? matchup?.Items ?? [];

  const winnerItemIdRaw =
    matchup?.winner_item_id ?? matchup?.winnerItemId ?? matchup?.WinnerItemID ?? null;

  const winnerItemId =
    winnerItemIdRaw !== null && winnerItemIdRaw !== undefined
      ? Number(winnerItemIdRaw)
      : null;

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

  const isReady = matchupStatus === "completed";

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

    if (!window.confirm("Delete this matchup?")) return;

    try {
      setIsDeleting(true);
      await deleteMatchup(matchup.id);
      navigate(`/users/${uid}/profile`);
    } catch (err) {
      console.error(err);
      setError("Unable to delete matchup.");
    } finally {
      setIsDeleting(false);
    }
  };

  const handleActivateMatchup = async () => {
    if (!window.confirm("Activate this matchup and start voting?")) return;
    await activateMatchup(matchup.id);
    await refreshMatchup();
  };
  
  

  const handleLikeToggle = async () => {
    if (!matchup || likePending || interactionLocked) return;

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
      setError("Unable to update like.");
    } finally {
      setLikePending(false);
    }
  };

  const handleCommentSubmit = async (e) => {
    e.preventDefault();
    if (!newComment.trim() || commentPending || interactionLocked) return;

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

  const handleOverrideWinner = async (winnerId) => {
    if (!canOverrideWinner) return;

    // If tie-after-expiry in active round, confirm that winner selection will advance
    const msg = isTieAfterExpiryInActiveRound
      ? "Select this winner? You can still change it until the round advances."
      : "Override votes and select this winner?";

    if (!window.confirm(msg)) return;

    try {
      await overrideMatchupWinner(matchup.id, winnerId);
      await refreshMatchup();
    } catch (err) {
      console.error(err);
      setError("Failed to select winner.");
    }
  };

  const handleReadyUp = async () => {
    if (!readyEnabled || readyPending || matchupExpired) return;

    const promptMessage = isReady ? "Undo ready?" : "Ready up and lock the winner?";
    if (!window.confirm(promptMessage)) return;

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
  };

  /* ------------------------------------------------------------------ */
  /* RENDER */
  /* ------------------------------------------------------------------ */

  if (isLoading) {
    return (
      <div className="matchup-page">
        <NavigationBar />
        <main className="matchup-content">
          <div className="matchup-status-card">Loading matchup…</div>
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

        <section className="matchup-section matchup-section--status">
          <div className="matchup-status-row">
            <div className="matchup-status-pill">
              Status: <strong>{matchupStatus || "unknown"}</strong>
            </div>

            {isTieAfterExpiryInActiveRound && (
              <div className="matchup-status-banner matchup-status-banner--warning">
                ⚠️ Voting ended in a tie. The owner must choose a winner before the round can advance.
              </div>
            )}

            {matchupEndsAt && !matchupExpired && (
              <div className="matchup-status-pill matchup-status-pill--timer">
                ⏳ Time remaining: <strong>{countdown.formatted}</strong>
              </div>
            )}

            {matchupExpired && (
              <div className="matchup-status-pill matchup-status-pill--locked">
                ⛔ Voting closed
              </div>
            )}
          </div>

          <div className="matchup-actions">
            <button
              type="button"
              className={`matchup-like-button ${isLiked ? "is-liked" : ""}`}
              onClick={handleLikeToggle}
              disabled={!canLike || likePending}
            >
              {likePending ? "Updating…" : isLiked ? "Liked" : "Like"}
            </button>
            <div className="matchup-like-indicator">
              <span className="matchup-like-count">{likesCount}</span>
              <span>Likes</span>
            </div>
          </div>

          {showActivateButton && (
            <div className="matchup-activate">
              <Button
                onClick={handleActivateMatchup}
                className="matchup-primary-button"
              >
                Activate matchup
              </Button>
            </div>
          )}


          {canReadyUp && (
            <div className="matchup-readyup">
              <Button
                onClick={handleReadyUp}
                disabled={!readyEnabled || readyPending || matchupExpired}
                className={`matchup-readyup-button ${
                  isReady ? "matchup-readyup-button--armed" : ""
                }`}
              >
                {readyPending ? (isReady ? "Unlocking…" : "Locking…") : isReady ? "Undo Ready" : "Ready up"}
              </Button>
            </div>
          )}
        </section>

        <section className="matchup-section">
          <header className="matchup-section-header">
            <h2>
              Matchup{" "}
              {!isBracketMatchup && (
                <>
                  · <Link to={`/users/${matchup.author_id}/profile`}>{authorName}</Link>
                </>
              )}
            </h2>

            {isVotingLocked && (
              <span className="matchup-badge matchup-badge--locked">
                Voting locked
              </span>
            )}
          </header>

          <div className="matchup-items">
            {items.map((item) => (
              <MatchupItem
                key={item.id}
                item={item}
                isWinner={winnerItemId === item.id}
                hasWinner={winnerItemId !== null}
                allowEdit={
                  isOwner &&
                  (!isBracketMatchup || bracket?.status === "draft")
                }
                isVotingLocked={isVotingLocked}
                isBracketMatchup={isBracketMatchup}
                canOverrideWinner={canOverrideWinner}
                disabled={isVotingLocked}
                onOverrideWinner={() => handleOverrideWinner(item.id)}
                onVote={refreshMatchup}
                isOwner={isOwner}
              />
            ))}
          </div>

          {isOwner && !isBracketMatchup && (
            <div style={{ marginTop: 16 }}>
              <Button
                onClick={handleDelete}
                disabled={isDeleting}
                className="matchup-danger-button"
              >
                {isDeleting ? "Deleting…" : "Delete matchup"}
              </Button>
            </div>
          )}
        </section>

        <section className="matchup-section">
          <header className="matchup-section-header">
            <h2>Comments</h2>
          </header>

          <div className="matchup-comments">
            {comments.length === 0 ? (
              <div className="matchup-empty">No comments yet.</div>
            ) : (
              comments.map((comment) => <Comment key={comment.id} comment={comment} />)
            )}
          </div>

          {interactionLocked && (
            <div className="matchup-inline-hint">
              Comments are disabled for expired or inactive matchups.
            </div>
          )}

          <form className="matchup-comment-form" onSubmit={handleCommentSubmit}>
            <label htmlFor="newComment" className="matchup-form-label">
              Add a comment
            </label>
            <textarea
              id="newComment"
              value={newComment}
              onChange={(e) => setNewComment(e.target.value)}
              rows={3}
              disabled={commentPending || interactionLocked}
              className="matchup-textarea"
            />
            {commentError && <p className="matchup-inline-error">{commentError}</p>}
            <div className="matchup-form-actions">
              <button
                type="submit"
                disabled={commentPending || interactionLocked}
                className="matchup-primary-button"
              >
                {commentPending ? "Posting…" : "Post comment"}
              </button>
            </div>
          </form>
        </section>
      </main>
    </div>
  );
};

export default MatchupPage;
