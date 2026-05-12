import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { motion } from "framer-motion";
import NavigationBar from "../components/NavigationBar";
import ConfirmModal from "../components/ConfirmModal";
import Button from "../components/Button";
import BracketView from "../components/BracketView";
import Comment from "../components/Comment";
import ShareButton from "../components/ShareButton";
import ReportModal from "../components/ReportModal";
import SkeletonCard from "../components/SkeletonCard";
import ProfilePic from "../components/ProfilePic";
import { relativeTime } from "../utils/time";
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
import useShareTracking from "../hooks/useShareTracking";
import { useAnonUpgradePrompt } from "../contexts/AnonUpgradeContext";

export default function BracketPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const viewerId = localStorage.getItem("userId");
  const { promptUpgrade } = useAnonUpgradePrompt();

  // Anon viewers can mount this page (RequireAuth was removed at the
  // route level so we can show a friendly modal instead of bouncing
  // them to /login). The modal explains why they need to sign up;
  // dismissing it routes them out via its own onClose handler.
  useEffect(() => {
    if (!viewerId) {
      promptUpgrade('bracket');
    }
    // Run once on mount per session — the modal itself collapses
    // duplicate triggers, but the empty deps array makes the intent
    // explicit (we don't want this firing on every viewerId-derived
    // re-render).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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
  const [deleteModalOpen, setDeleteModalOpen] = useState(false);
  const [isDeleting, setIsDeleting] = useState(false);
  const [reportOpen, setReportOpen] = useState(false);
  const [confirmModal, setConfirmModal] = useState(null);
  const [commentError, setCommentError] = useState(null);
  const [commentPending, setCommentPending] = useState(false);

  // Attribution on incoming shares (see hook).
  useShareTracking({ contentType: "bracket", shortID: bracket?.short_id });

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
      const summary = summaryRes.data?.summary ?? summaryRes.data?.response ?? summaryRes.data ?? {};
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
      const raw = res.data?.comments ?? res.data?.response ?? res.data;
      setComments(Array.isArray(raw) ? raw : []);
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
        setCurrentUser(res.data?.user ?? res.data?.response ?? res.data ?? null);
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

  // Listen for server-sent bracket advance events to refresh without polling.
  useEffect(() => {
    if (bracketAdvanceMode !== "timer" || bracket?.status !== "active" || !bracket?.id) return;

    const es = new EventSource(`/brackets/${bracket.id}/events`);
    es.onmessage = (e) => {
      if (e.data === "advance") loadBracket({ silent: true });
    };
    es.onerror = () => es.close();

    return () => es.close();
  }, [bracketAdvanceMode, bracket?.status, bracket?.id, loadBracket]);

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

  const canEdit = isOwner || currentUser?.is_admin === true;

  /* ------------------------------------------------------------------ */
  /* ACTIONS */
  /* ------------------------------------------------------------------ */

  const handleActivate = () => {
    if (!bracket) return;
    setConfirmModal({
      message: 'Activate this bracket?',
      confirmLabel: 'Activate',
      danger: false,
      onConfirm: async () => {
        await updateBracket(bracket.id, {
          title: bracket.title,
          description: bracket.description,
          status: "active",
        });
        await loadBracket();
      },
    });
  };

  const handleAdvance = async () => {
    if (!bracket) return;
    setError(null);
    try {
      await advanceBracket(bracket.id);
      await loadBracket();
    } catch (err) {
      // Map the backend's verbose error strings to a friendly inline
      // message rather than letting an uncaught exception surface as
      // a console error. The "not completed" branch is the common
      // case — user clicked Advance before all current-round
      // matchups finished.
      const msg = err?.response?.data?.message || err?.message || '';
      if (msg.includes('not completed')) {
        setError("All matches in the current round haven't been completed yet.");
      } else if (msg.includes('already populated')) {
        setError('This round has already been advanced.');
      } else {
        setError("Couldn't advance the round. Please try again.");
      }
    }
  };

  const handleDelete = async () => {
    if (!bracket) return;
    try {
      setIsDeleting(true);
      await deleteBracket(bracket.id);
      const authorSlug = bracket?.author?.username ?? bracket?.author_id ?? bracket?.authorId;
      navigate(authorSlug ? `/users/${authorSlug}` : "/home");
    } catch (err) {
      console.error(err);
      setError("Unable to delete bracket.");
    } finally {
      setIsDeleting(false);
    }
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
        {/* Twitter-style hero card. Two-column grid: owner avatar on the
            left, byline + title + description + meta on the right.
            Status moves out of the meta row into a small pill anchored
            top-right (pulses green when active). The previous overline
            "Tournament snapshot · by @x" is replaced with a stacked
            display-name + @handle + relative-timestamp block. */}
        <motion.section
          className="bracket-hero-summary"
          aria-label="Bracket overview"
          {...sectionMotion}
        >
          {(() => {
            const ownerUsername = bracket.author?.username || bracket.author_username;
            const ownerId = bracket.author?.id || bracket.author_id || bracket.authorId;
            const displayName = bracket.author?.display_name || ownerUsername;
            const profileSlug = ownerUsername || ownerId;
            const state = String(bracket.status || "").toLowerCase();
            const statusLabel = state ? state.toUpperCase() : "UNKNOWN";
            const createdAt = bracket.created_at || bracket.createdAt;
            return (
              <>
                {/* Status pill — top-right corner. data-state drives the
                    color treatment + dot animation via CSS. role="status"
                    so screen readers announce changes if the bracket
                    transitions live. */}
                <span
                  className="bracket-status-pill"
                  data-state={state || "unknown"}
                  role="status"
                >
                  <span className="bracket-status-pill__dot" aria-hidden="true" />
                  <span className="bracket-status-pill__label">{statusLabel}</span>
                </span>

                {/* Avatar — left column. ProfilePic resolves the S3 path
                    from the user record; the outer <Link> carries the
                    accessible name so the inner <img> stays decorative. */}
                {profileSlug && (
                  <Link
                    to={`/users/${profileSlug}`}
                    className="bracket-hero-avatar"
                    aria-label={`${displayName || "Owner"} profile`}
                  >
                    {ownerId ? (
                      <ProfilePic userId={ownerId} size={96} />
                    ) : (
                      <span className="bracket-hero-avatar__fallback" aria-hidden="true">
                        {(displayName || "?").charAt(0).toUpperCase()}
                      </span>
                    )}
                  </Link>
                )}

                <div className="bracket-hero-text">
                  <header className="bracket-overline">
                    {ownerUsername ? (
                      <Link
                        to={`/users/${profileSlug}`}
                        className="bracket-byline-name"
                      >
                        {displayName}
                      </Link>
                    ) : (
                      <span className="bracket-byline-name">{displayName || "Unknown"}</span>
                    )}
                    <span className="bracket-byline-meta">
                      {ownerUsername && (
                        <Link
                          to={`/users/${profileSlug}`}
                          className="bracket-byline-handle"
                        >
                          @{ownerUsername}
                        </Link>
                      )}
                      {createdAt && (
                        <>
                          {ownerUsername && <span aria-hidden="true"> · </span>}
                          <time
                            className="bracket-byline-time"
                            dateTime={createdAt}
                            title={new Date(createdAt).toLocaleString()}
                          >
                            {relativeTime(createdAt)}
                          </time>
                        </>
                      )}
                    </span>
                  </header>

                  <h1 className="bracket-hero-title">{bracket.title}</h1>
                  <p className="bracket-description">
                    {bracket.description || "No description provided yet."}
                  </p>
                </div>

                <div className="bracket-meta-row">
                  <div className="bracket-meta-item">
                    <span className="bracket-meta-label">Current round</span>
                    <p className="bracket-meta-value">
                      {bracket.current_round || 1}
                    </p>
                  </div>
                </div>
              </>
            );
          })()}

          {bracket.status === "active" && roundEndsAt && (
            <div className="bracket-round-timer">
              ⏳ Round ends in{" "}
              <strong>{roundCountdown.formatted}</strong>
            </div>
          )}
        </motion.section>

        {/* Action bar: split into two regions so public engagement
            actions (Like, Share) sit visually + structurally apart
            from owner-only management actions (Advance, Delete). The
            previous three-group flex pill placed gradient-Delete next
            to gradient-Advance with equal weight — a misclick hazard.
            Delete is demoted to a ghost outline; Advance/Activate
            stays the primary, right-anchored CTA. Tab order is Like
            → Share → Delete → Advance, so keyboard users hit the
            destructive action BEFORE the primary one (forces a
            deliberate tab to advance, makes "I meant to delete"
            harder to mis-press). */}
        <motion.section
          className="bracket-action-bar"
          aria-label="Bracket actions"
          {...sectionMotion}
        >
          <div className="bracket-action-bar__engagement">
            {viewerId && (
              <button
                type="button"
                className={`bracket-like-button ${isLiked ? "is-liked" : ""}`}
                aria-pressed={isLiked}
                aria-label={isLiked
                  ? `Unlike (${likesCount} likes)`
                  : `Like (${likesCount} likes)`}
                onClick={handleLikeToggle}
                disabled={likePending}
              >
                <span aria-hidden="true" className="bracket-like-button__icon">
                  {isLiked ? "♥" : "♡"}
                </span>
                <span className="bracket-like-button__label">
                  {likePending ? "Updating…" : isLiked ? "Liked" : "Like"}
                </span>
                <span className="bracket-like-button__count">· {likesCount}</span>
              </button>
            )}
            {!viewerId && (
              <span className="bracket-like-indicator" aria-label={`${likesCount} likes`}>
                <span className="bracket-like-count">{likesCount}</span>
                <span>Likes</span>
              </span>
            )}
            <ShareButton item={bracket} type="bracket" />
            {/* Report button removed at user request — backend handler
                + ReportModal mount remain so re-enabling is one-line. */}
          </div>

          {/* Owner region — fully absent from the DOM for non-owners
              (intentional per spec: not a display:none hide, so screen
              readers and keyboard users don't tab to controls they
              can't use). */}
          {canEdit && (
            <div className="bracket-action-bar__owner">
              <button
                type="button"
                onClick={() => setDeleteModalOpen(true)}
                disabled={isDeleting}
                className="bracket-button bracket-button--danger bracket-button--ghost"
              >
                {isDeleting ? "Deleting…" : "Delete"}
              </button>
              {bracket.status === "draft" && (
                <button
                  type="button"
                  onClick={handleActivate}
                  className="bracket-button bracket-button--primary"
                >
                  Activate bracket
                </button>
              )}
              {bracket.status === "active" && (
                <button
                  type="button"
                  onClick={handleAdvance}
                  className="bracket-button bracket-button--primary"
                >
                  Advance to next round
                </button>
              )}
            </div>
          )}

          {deleteModalOpen && (
            <div className="edit-profile-overlay" onClick={() => setDeleteModalOpen(false)}>
              <div className="edit-profile-modal" onClick={(e) => e.stopPropagation()}>
                <h2 className="edit-profile-title">Delete bracket?</h2>
                <p style={{ color: 'rgba(226,232,240,0.7)', fontSize: '0.9rem', margin: 0 }}>
                  This will delete the bracket and all its matchups. This can't be undone.
                </p>
                <div className="edit-profile-actions">
                  <Button className="bracket-button" onClick={() => setDeleteModalOpen(false)} disabled={isDeleting}>
                    Cancel
                  </Button>
                  <Button className="bracket-button bracket-button--danger" onClick={() => { setDeleteModalOpen(false); handleDelete(); }} disabled={isDeleting}>
                    {isDeleting ? 'Deleting…' : 'Delete'}
                  </Button>
                </div>
              </div>
            </div>
          )}
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
          </header>

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
                  subjectType="bracket_comment"
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
        </motion.section>
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

      {reportOpen && bracket?.public_id && (
        <ReportModal
          subjectType="bracket"
          subjectId={bracket.public_id}
          onClose={() => setReportOpen(false)}
        />
      )}
    </div>
  );
}
