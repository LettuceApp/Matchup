import { useCallback, useEffect, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { motion } from "framer-motion";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import BracketView from "../components/BracketView";
import Comment from "../components/Comment";
import SkeletonCard from "../components/SkeletonCard";
import {
  getBracketSummary,
  getBracketComments,
  getCurrentUser,
  updateBracket,
  advanceBracket,
  deleteBracket,
  likeBracket,
  unlikeBracket,
  createBracketComment,
  deleteBracketComment,
} from "../services/api";
import "../styles/BracketPage.css";
import useCountdown from "../hooks/useCountdown";

export default function BracketPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const viewerId = localStorage.getItem("userId");

  const [bracket, setBracket] = useState(null);
  const [matchups, setMatchups] = useState([]);
  const [champion, setChampion] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentUser, setCurrentUser] = useState(null);
  const [likesCount, setLikesCount] = useState(0);
  const [isLiked, setIsLiked] = useState(false);
  const [likePending, setLikePending] = useState(false);
  const [comments, setComments] = useState([]);
  const [newComment, setNewComment] = useState("");
  const [commentError, setCommentError] = useState(null);
  const [commentPending, setCommentPending] = useState(false);
  const [commentsOpen, setCommentsOpen] = useState(false);
  const loadInFlight = useRef(false);

  /* ------------------------------------------------------------------ */
  /* DATA LOADING */
  /* ------------------------------------------------------------------ */

  const loadBracket = useCallback(async (options = {}) => {
    const { silent = false } = options;
    if (loadInFlight.current) return;
    loadInFlight.current = true;
    if (!silent) {
      setLoading(true);
      setError(null);
    }

    try {
      const summaryRes = await getBracketSummary(id, viewerId);
      const summary = summaryRes.data?.response ?? summaryRes.data ?? {};
      const bracketData = summary.bracket ?? null;
      const matchupsData = summary.matchups ?? [];
      setBracket(bracketData);
      setMatchups(Array.isArray(matchupsData) ? matchupsData : []);
      detectChampion(bracketData, matchupsData);
      setLikesCount(
        Number(bracketData?.likes_count ?? bracketData?.likesCount ?? 0),
      );
      setIsLiked(Boolean(summary.liked_bracket));
    } catch (err) {
      console.error("Failed to load bracket", err);
      if (!silent) {
        setError(
          err?.response?.status === 403
            ? "Only followers can view this bracket."
            : "We could not load this bracket right now."
        );
        setBracket(null);
        setMatchups([]);
        setChampion(null);
      }
    } finally {
      if (!silent) {
        setLoading(false);
      }
      loadInFlight.current = false;
    }
  }, [id, viewerId]);

  useEffect(() => {
    loadBracket();
  }, [loadBracket]);

  const loadComments = useCallback(async () => {
    try {
      const res = await getBracketComments(id);
      setComments(res.data?.response ?? res.data ?? []);
    } catch (err) {
      console.error("Failed to load bracket comments", err);
      if (err?.response?.status === 403) {
        setCommentError("Only followers can view bracket comments.");
      }
      setComments([]);
    }
  }, [id]);

  useEffect(() => {
    loadComments();
  }, [loadComments]);

  useEffect(() => {
    const loadMe = async () => {
      try {
        const res = await getCurrentUser();
        setCurrentUser(res.data?.response ?? res.data ?? null);
      } catch (err) {
        console.warn("Unable to load current user", err);
      }
    };
    loadMe();
  }, []);

  useEffect(() => {
    if (!viewerId) {
      setIsLiked(false);
    }
  }, [viewerId]);

  /* ------------------------------------------------------------------ */
  /* TIMER (ROUND COUNTDOWN) */
  /* ------------------------------------------------------------------ */

  const bracketAdvanceMode =
    bracket?.advance_mode ?? bracket?.advanceMode ?? "manual";
  const roundEndsAt =
    bracketAdvanceMode === "timer"
      ? bracket?.round_ends_at ?? bracket?.roundEndsAt ?? null
      : null;

  const roundCountdown = useCountdown(roundEndsAt);

  // Poll for timer-driven advances so champion + round updates appear without refresh.
  useEffect(() => {
    if (bracketAdvanceMode !== "timer" || bracket?.status !== "active") return;

    const intervalMs = roundCountdown.isExpired ? 3000 : 10000;
    const interval = setInterval(() => {
      loadBracket({ silent: true });
    }, intervalMs);

    return () => clearInterval(interval);
  }, [
    bracketAdvanceMode,
    bracket?.status,
    roundCountdown.isExpired,
    loadBracket,
  ]);

  /* ------------------------------------------------------------------ */
  /* CHAMPION DETECTION */
  /* ------------------------------------------------------------------ */

  const detectChampion = (bracketData, rawMatchups) => {
    if (!bracketData || bracketData.status !== "completed") {
      setChampion(null);
      return;
    }

    const list = Array.isArray(rawMatchups)
      ? rawMatchups
      : Array.isArray(bracketData.matchups)
      ? bracketData.matchups
      : [];

    if (!list.length) {
      setChampion(null);
      return;
    }

    const rounds = list.map((m) => Number(m.round || 0));
    const lastRound = rounds.length ? Math.max(...rounds) : 0;

    const finalMatch = list.find(
      (m) => Number(m.round) === lastRound && m.winner_item_id
    );

    if (!finalMatch) return;

    const winnerItem = finalMatch.items?.find(
      (i) => i.id === finalMatch.winner_item_id
    );

    if (winnerItem) {
      setChampion({
        matchupId: finalMatch.id,
        item: winnerItem,
        round: lastRound,
      });
    }
  };

  /* ------------------------------------------------------------------ */
  /* LOADING / ERROR STATES */
  /* ------------------------------------------------------------------ */

  if (loading) {
    return (
      <div className="bracket-page">
        <NavigationBar />
        <main className="bracket-content">
          <div className="bracket-skeleton-grid">
            <SkeletonCard lines={3} />
            <SkeletonCard lines={2} />
          </div>
        </main>
      </div>
    );
  }

  if (!bracket || error) {
    return (
      <div className="bracket-page">
        <NavigationBar />
        <main className="bracket-content">
          <div className="bracket-status-card bracket-status-card--error">
            {error || "Bracket not found."}
          </div>
        </main>
      </div>
    );
  }

  /* ------------------------------------------------------------------ */
  /* PERMISSIONS */
  /* ------------------------------------------------------------------ */

  const bracketOwnerId =
    bracket?.author_id ??
    bracket?.authorId ??
    bracket?.author?.id ??
    null;

  const isOwner =
    currentUser?.id &&
    bracketOwnerId &&
    String(currentUser.id) === String(bracketOwnerId);

  const canEdit = isOwner || currentUser?.role === "admin";

  /* ------------------------------------------------------------------ */
  /* ACTIONS */
  /* ------------------------------------------------------------------ */

  const handleActivate = async () => {
    if (!bracket) return;
    if (!window.confirm("Activate this bracket?")) return;

    await updateBracket(bracket.id, {
      title: bracket.title,
      description: bracket.description,
      status: "active",
    });

    await loadBracket();
  };

  const handleAdvance = async () => {
    if (!bracket) return;
    await advanceBracket(bracket.id);
    await loadBracket();
  };

  const handleDelete = async () => {
    if (!bracket) return;
    if (!window.confirm("Delete this bracket?")) return;

    await deleteBracket(bracket.id);
    navigate("/home");
  };

  const handleCommentSubmit = async (e) => {
    e.preventDefault();
    if (!newComment.trim() || commentPending || !viewerId) return;

    try {
      setCommentPending(true);
      setCommentError(null);
      await createBracketComment(id, { body: newComment.trim() });
      setNewComment("");
      await loadComments();
    } catch (err) {
      console.error("Unable to post bracket comment", err);
      setCommentError("Unable to post comment.");
    } finally {
      setCommentPending(false);
    }
  };

  const handleLikeToggle = async () => {
    if (!bracket || likePending || !viewerId) return;

    try {
      setLikePending(true);
      if (isLiked) {
        await unlikeBracket(bracket.id);
        setIsLiked(false);
        setLikesCount((count) => Math.max(0, count - 1));
      } else {
        await likeBracket(bracket.id);
        setIsLiked(true);
        setLikesCount((count) => count + 1);
      }
    } catch (err) {
      const message = err?.response?.data?.error ?? err?.message ?? "";
      if (typeof message === "string" && message.includes("already liked")) {
        setIsLiked(true);
      } else {
        console.error("Unable to update bracket like", err);
        setError("Unable to update like.");
      }
    } finally {
      setLikePending(false);
    }
  };

  /* ------------------------------------------------------------------ */
  /* RENDER */
  /* ------------------------------------------------------------------ */

  const sectionMotion = {
    initial: { opacity: 0, y: 14 },
    animate: { opacity: 1, y: 0 },
    transition: { duration: 0.35 },
  };

  return (
    <div className="bracket-page">
      <NavigationBar />
      <main className="bracket-content">
        <motion.section className="bracket-hero-summary" {...sectionMotion}>
          <p className="bracket-overline">Tournament snapshot</p>
          <h1>{bracket.title}</h1>
          <p className="bracket-description">
            {bracket.description || "No description provided yet."}
          </p>

          <div className="bracket-meta-row">
            <div>
              <span className="bracket-meta-label">Status</span>
              <p className="bracket-status-pill">
                {(bracket.status || "draft").toUpperCase()}
              </p>
            </div>

            <div>
              <span className="bracket-meta-label">Current round</span>
              <p className="bracket-meta-value">
                {bracket.current_round || 1}
              </p>
            </div>
          </div>

          {bracket.status === "active" && roundEndsAt && (
            <div className="bracket-round-timer">
              ⏳ Round ends in{" "}
              <strong>{roundCountdown.formatted}</strong>
            </div>
          )}
        </motion.section>

        <motion.section className="bracket-action-bar" {...sectionMotion}>
          <div className="bracket-action-group">
            {viewerId ? (
              <button
                type="button"
                className={`bracket-like-button ${isLiked ? "is-liked" : ""}`}
                onClick={handleLikeToggle}
                disabled={likePending}
              >
                {likePending ? "Updating…" : isLiked ? "Liked" : "Like"}
              </button>
            ) : null}
            <div className="bracket-like-indicator">
              <span className="bracket-like-count">{likesCount}</span>
              <span>Likes</span>
            </div>
          </div>

          <div className="bracket-action-group bracket-action-group--center">
            {canEdit && bracket.status === "draft" && (
              <Button onClick={handleActivate} className="bracket-button">
                Activate bracket
              </Button>
            )}

            {canEdit && bracket.status === "active" && (
              <Button onClick={handleAdvance} className="bracket-button">
                Advance to next round
              </Button>
            )}

            {!canEdit && (
              <span className="bracket-action-hint">
                Votes happening now · follow the winners below
              </span>
            )}
          </div>

          <div className="bracket-action-group bracket-action-group--right">
            {canEdit && (
              <Button
                onClick={handleDelete}
                className="bracket-button bracket-button--danger"
              >
                Delete
              </Button>
            )}
          </div>
        </motion.section>

        <motion.section className="bracket-section bracket-section--stage" {...sectionMotion}>
          <header className="bracket-section-header">
            <div>
              <h2>Bracket matches</h2>
              <p>Watch each round resolve as winners advance.</p>
            </div>
          </header>

          <BracketView
            matchups={matchups}
            bracket={bracket}
            champion={champion}
          />
        </motion.section>

        <motion.section
          className="bracket-section bracket-section--comments"
          {...sectionMotion}
        >
          <header className="bracket-section-header bracket-section-header--comments">
            <div>
              <h2>Comments</h2>
              <p>Join the debate.</p>
            </div>
            <button
              type="button"
              className="bracket-comment-toggle"
              onClick={() => setCommentsOpen((prev) => !prev)}
            >
              {commentsOpen ? "Hide" : "Show"}
            </button>
          </header>

          {commentsOpen && (
            <>
              <div className="bracket-comments">
                {comments.length === 0 ? (
                  <div className="bracket-comment-empty">No comments yet.</div>
                ) : (
                  comments.map((comment) => (
                    <Comment
                      key={comment.id}
                      comment={comment}
                      refreshComments={loadComments}
                      onDelete={deleteBracketComment}
                    />
                  ))
                )}
              </div>

              {viewerId ? (
                <form className="bracket-comment-form" onSubmit={handleCommentSubmit}>
                  <label htmlFor="newBracketComment" className="bracket-form-label">
                    Add a comment
                  </label>
                  <textarea
                    id="newBracketComment"
                    value={newComment}
                    onChange={(e) => setNewComment(e.target.value)}
                    rows={3}
                    disabled={commentPending}
                    className="bracket-textarea"
                  />
                  {commentError && (
                    <p className="bracket-inline-error">{commentError}</p>
                  )}
                  <div className="bracket-form-actions">
                    <button
                      type="submit"
                      disabled={commentPending}
                      className="bracket-button bracket-button--ghost"
                    >
                      {commentPending ? "Posting…" : "Post comment"}
                    </button>
                  </div>
                </form>
              ) : null}
            </>
          )}
        </motion.section>
      </main>
    </div>
  );
}
