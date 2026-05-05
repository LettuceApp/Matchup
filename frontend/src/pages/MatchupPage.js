import React, { useState, useEffect, useCallback, useRef } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { FiHeart, FiMessageCircle, FiShare2, FiFlag } from "react-icons/fi";
import NavigationBar from "../components/NavigationBar";
import MatchupItem from "../components/MatchupItem";
import AnonVoteCounter from "../components/AnonVoteCounter";
import { useAnonVoteStatus } from "../hooks/useAnonVoteStatus";
import { useAnonUpgradePrompt } from "../contexts/AnonUpgradeContext";
import { track } from "../utils/analytics";
import Comment from "../components/Comment";
import Button from "../components/Button";
import ConfirmModal from "../components/ConfirmModal";
import ShareButton from "../components/ShareButton";
import ReportModal from "../components/ReportModal";
import SkeletonCard from "../components/SkeletonCard";
import Reveal from "../components/Reveal";
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
import useShareTracking from "../hooks/useShareTracking";
import { relativeTime } from "../utils/time";

// relativeTime moved to utils/time.js; imported below.

const MatchupPage = () => {
  const { uid, id } = useParams();
  const navigate = useNavigate();
  const userId = localStorage.getItem("userId");
  const viewerId = userId || null;

  // Anon-only vote-counter state. The hook short-circuits internally
  // when no anon UUID exists yet, so we can mount it unconditionally
  // even though the chip is hidden for authed users.
  const anonVoteStatus = useAnonVoteStatus();
  const { promptUpgrade } = useAnonUpgradePrompt();

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
  // Report flow — non-owners can flag a matchup for moderator review.
  const [reportOpen, setReportOpen] = useState(false);

  // Fire the share-attribution beacon on landing. Runs once per mount
  // after short_id is known; safe when short_id is missing (no-op).
  useShareTracking({ contentType: "matchup", shortID: matchup?.short_id });

  // Resolve the viewer's currently-selected item label for share-text
  // personalization ("I'm team {label} — you?"). Lazily derived because
  // both matchup.items and votedItemId can update independently.
  const viewerVoteForShare = (() => {
    if (!matchup?.items || !votedItemId) return null;
    const item = matchup.items.find((it) => it.id === votedItemId);
    if (!item) return null;
    return { itemLabel: item.item };
  })();

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

  // Lookup the winning contender object for the reveal banner. Null
  // until status=completed (via displayWinnerId gating).
  const winnerItem =
    displayWinnerId !== null
      ? items.find((it) => it.id === displayWinnerId) ?? null
      : null;

  // "2d ago" for the hero overline. relativeTime gracefully returns ""
  // if created_at is missing, in which case the overline omits the dot.
  const timeAgo = relativeTime(matchup?.created_at);

  // Aggregate comment count for the engagement line + action bar.
  const commentsCount = Array.isArray(comments) ? comments.length : 0;

  // Whether any owner action is available on this matchup — drives
  // visibility of the whole owner tray so a plain viewer never sees it.
  const hasOwnerActions =
    canReadyUp ||
    canActivate ||
    canOverrideWinner ||
    (isOwner && !isBracketMatchup);

  // Title + description shown in the hero. For bracket matchups, use
  // the PARENT bracket's title as the H1 so users instantly know the
  // tournament they're voting in — the round and specific pairing are
  // clear from the overline and the contender tiles below. Fall back
  // to the matchup's own title if the bracket hasn't loaded yet.
  // Optional chaining because these evaluate before the early-return
  // for null matchup (loading state).
  const heroTitle =
    isBracketMatchup && bracket?.title ? bracket.title : matchup?.title;
  const heroDescription = isBracketMatchup
    ? bracket?.description
    : matchup?.content;
  const roundLabel =
    isBracketMatchup && matchup?.round ? `Round ${matchup.round}` : null;

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
        track('matchup_unliked', { matchup_id: matchup.id });
      } else {
        await likeMatchup(matchup.id);
        setIsLiked(true);
        setLikesCount((c) => c + 1);
        track('matchup_liked', { matchup_id: matchup.id });
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
      track('comment_created', { matchup_id: id });
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

        {isBracketMatchup && bracket && (
          <div className="matchup-bracket-crumb">
            <Link to={`/brackets/${matchup.bracket_id}`} className="matchup-bracket-crumb__link">
              ← {bracket.title || "Back to bracket"}
            </Link>
          </div>
        )}

        {/* Hero: cover image (inset), overline, H1, description, tags,
            aggregate engagement, status row. Single-column stacked so
            the title never plays second fiddle. */}
        <section className="matchup-hero">
          {matchup.image_url && (
            <div className="matchup-hero__media">
              <img src={matchup.image_url} alt={matchup.title} decoding="async" />
            </div>
          )}
          <div className="matchup-hero__body">
            <p className="matchup-overline">
              {isBracketMatchup ? "Tournament" : "Matchup"}
              {" · by "}
              <Link
                to={`/users/${matchup?.author?.username || matchup.author_id}`}
                className="matchup-author-link"
              >
                @{authorName}
              </Link>
              {roundLabel && (
                <>
                  {" · "}
                  <span className="matchup-overline__round">{roundLabel}</span>
                </>
              )}
              {timeAgo && (
                <>
                  {" · "}
                  <span className="matchup-overline__time">{timeAgo}</span>
                </>
              )}
            </p>

            <h1>{heroTitle}</h1>

            {heroDescription && (
              <p className="matchup-description">{heroDescription}</p>
            )}

            {Array.isArray(matchup.tags) && matchup.tags.length > 0 && (
              <div className="matchup-tag-row">
                {matchup.tags.map((tag) => (
                  <span key={tag} className="matchup-tag">{tag}</span>
                ))}
              </div>
            )}

            <div className="matchup-engagement">
              <span><strong>{totalVotes}</strong> {totalVotes === 1 ? "vote" : "votes"}</span>
              <span className="matchup-engagement__dot">·</span>
              <span><strong>{likesCount}</strong> {likesCount === 1 ? "like" : "likes"}</span>
              <span className="matchup-engagement__dot">·</span>
              <span><strong>{commentsCount}</strong> {commentsCount === 1 ? "comment" : "comments"}</span>
            </div>

            <div className="matchup-status-row">
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
              {/* Status pill hides when active — countdown already implies it. */}
              {matchupStatus && matchupStatus !== "active" && !matchupExpired && (
                <div className="matchup-status-pill">
                  Status: <strong>{matchupStatus}</strong>
                </div>
              )}
            </div>

            {isTieAfterExpiryInActiveRound && (
              <div className="matchup-status-banner matchup-status-banner--warning">
                ⚠️ Voting ended in a tie. The owner must choose a winner before the round can advance.
              </div>
            )}
          </div>
        </section>

        {/* Winner reveal — gets its own Reveal-animated banner so
            status=completed feels like an event, not a quiet flag. */}
        {isReady && winnerItem && (
          <Reveal as="section" className="matchup-winner-banner">
            <span className="matchup-winner-banner__trophy" aria-hidden="true">🏆</span>
            <span className="matchup-winner-banner__label">Winner</span>
            <strong className="matchup-winner-banner__name">
              {winnerItem.item ?? winnerItem.name ?? "Contender"}
            </strong>
          </Reveal>
        )}

        <section className="matchup-vote-stage" aria-label="Tap a contender to cast your vote">
          {/* Anon vote counter — visible only to non-signed-in
              viewers. Hidden when atCap so the AnonUpgradeModal
              triggered by the next vote attempt doesn't compete
              with the chip-version of the same CTA. */}
          {!viewerId && !isBracketMatchup && !anonVoteStatus.atCap && (
            <div className="matchup-anon-counter">
              <AnonVoteCounter
                used={anonVoteStatus.used}
                max={anonVoteStatus.max}
                atCap={false}
              />
            </div>
          )}
          {!viewerId && !isBracketMatchup && anonVoteStatus.atCap && (
            <div className="matchup-anon-counter">
              <AnonVoteCounter
                used={anonVoteStatus.used}
                max={anonVoteStatus.max}
                atCap={true}
                onPromptSignup={() => promptUpgrade('cap')}
              />
            </div>
          )}
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
                onVote={() => {
                  setVotedItemId(item.id);
                  if (!viewerId) {
                    // Anon successful-vote path — bump the counter
                    // chip immediately + sync against the server.
                    anonVoteStatus.bumpOptimistic();
                    anonVoteStatus.refresh();
                  }
                  return refreshMatchup();
                }}
                isOwner={isOwner}
                isVoted={votedItemId === item.id}
              />
            ))}
          </div>
        </section>

        {/* Flat action bar — Like / Comment / Share. Matches HomeCard's
            affordance so the same interaction pattern shows up across
            the app. Vote locked? Like is still available. */}
        <section className="matchup-action-bar" aria-label="Matchup actions">
          <button
            type="button"
            className={`matchup-action-bar__button ${isLiked ? "is-liked" : ""}`}
            onClick={handleLikeToggle}
            disabled={!viewerId || !canLike || likePending}
          >
            <FiHeart aria-hidden="true" />
            <span>{likePending ? "…" : isLiked ? "Liked" : "Like"}</span>
            {likesCount > 0 && <span className="matchup-action-bar__count">{likesCount}</span>}
          </button>
          <a
            href="#newComment"
            className="matchup-action-bar__button"
            onClick={(e) => {
              // Smooth-scroll + focus the textarea when possible; anchor
              // nav is the fallback for disabled JS / screen readers.
              const el = document.getElementById("newComment");
              if (el && el.scrollIntoView) {
                e.preventDefault();
                el.scrollIntoView({ behavior: "smooth", block: "center" });
                if (typeof el.focus === "function") el.focus({ preventScroll: true });
              }
            }}
          >
            <FiMessageCircle aria-hidden="true" />
            <span>Comment</span>
            {commentsCount > 0 && <span className="matchup-action-bar__count">{commentsCount}</span>}
          </a>
          <div className="matchup-action-bar__share">
            <ShareButton item={matchup} type="matchup" viewerVote={viewerVoteForShare} />
          </div>
          {/* Report lives in the action bar for non-owners only; owners
              don't need to flag their own content. Auth-gated so anon
              viewers don't get a dead button. */}
          {viewerId && !isOwner && (
            <button
              type="button"
              className="matchup-action-bar__button matchup-action-bar__button--subtle"
              aria-label="Report this matchup"
              onClick={() => setReportOpen(true)}
            >
              <FiFlag aria-hidden="true" />
              <span>Report</span>
            </button>
          )}
        </section>

        {reportOpen && matchup?.public_id && (
          <ReportModal
            subjectType="matchup"
            subjectId={matchup.public_id}
            onClose={() => setReportOpen(false)}
          />
        )}

        {/* Owner tray: consolidates Activate / Ready up / Override /
            Delete so rare-but-high-consequence actions don't compete
            with the viewer's vote CTA. Hidden entirely when no owner
            action applies. */}
        {hasOwnerActions && (
          <section className="matchup-owner-tray" aria-label="Owner controls">
            <p className="matchup-owner-tray__label">Owner controls</p>
            <div className="matchup-owner-tray__actions">
              {canActivate && (
                <Button
                  onClick={handleActivateMatchup}
                  disabled={activatePending}
                  className="matchup-owner-tray__button"
                >
                  {activatePending ? "Activating…" : "Activate matchup"}
                </Button>
              )}
              {canReadyUp && (
                <Button
                  onClick={handleReadyUp}
                  disabled={!readyEnabled || readyPending}
                  title="Closes voting & reveals winner"
                  className={`matchup-owner-tray__button ${isReady ? "is-armed" : ""}`}
                >
                  {readyPending ? (isReady ? "Unlocking…" : "Locking…") : isReady ? "Undo Ready" : "Ready up"}
                </Button>
              )}
              {canOverrideWinner && items.length > 0 && (
                <details ref={winnerMenuRef} className="matchup-winner-menu matchup-winner-menu--tray">
                  <summary className="matchup-winner-summary" aria-label="Override winner">
                    Override winner ⋯
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
              {isOwner && !isBracketMatchup && (
                <Button
                  onClick={() => setDeleteModalOpen(true)}
                  disabled={isDeleting}
                  className="matchup-owner-tray__button matchup-owner-tray__button--danger"
                >
                  {isDeleting ? "Deleting…" : "Delete matchup"}
                </Button>
              )}
              {isVotingLocked && (
                <span className="matchup-badge matchup-badge--locked">
                  Voting locked
                </span>
              )}
            </div>
          </section>
        )}

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
