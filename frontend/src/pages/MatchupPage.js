import React, { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useNavigate, useParams, Link } from "react-router-dom";
import { FiHeart, FiMessageCircle } from "react-icons/fi";
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
import ProfilePic from "../components/ProfilePic";
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
  getBracketMatchups,
  getMatchups,
  skipMatchup,
} from "../services/api";
import "../styles/MatchupPage.css";
import useCountdown from "../hooks/useCountdown";
import useShareTracking from "../hooks/useShareTracking";
import { relativeTime } from "../utils/time";
import { readHistory, writeHistory } from "../utils/matchupNavHistory";

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
  // Sibling matchups in the same bracket. Used to compute the
  // "Match X of Y · Round N" progress chip — pairwise-comparison
  // research is consistent that surfacing where you are in the
  // sequence reduces abandonment on multi-vote flows.
  const [bracketMatchups, setBracketMatchups] = useState([]);
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
  // Swipe-stream nav state. `history` is the ordered list of matchups
  // the viewer has hit in this detail-flow session; `cursor` is their
  // index within it. Persisted to sessionStorage by the reconciliation
  // effect below, so a tab refresh keeps place but a new tab starts
  // clean. navPending guards both buttons against double-tap while
  // the fetch + skip RPC are in flight.
  const [navState, setNavState] = useState(() => readHistory());
  const [navPending, setNavPending] = useState(false);
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
      // Anon viewers can't reach the bracket RPCs (the anon-bracket
      // gate added in 1e675c1 rejects with Unauthenticated). Without
      // this short-circuit, the page errored with the generic
      // "We couldn't load this matchup right now." banner. Instead,
      // show the anon-upgrade modal — the matchup itself stays
      // viewable; only the bracket-context fetches are skipped.
      if (!viewerId) {
        promptUpgrade('bracket');
        setBracket(null);
        setBracketMatchups([]);
      } else {
        // Fire both calls in parallel — neither depends on the other,
        // and the bracket-matchups list is what powers the
        // "Match X of Y · Round N" progress chip.
        const [b, ms] = await Promise.all([
          getBracket(matchupData.bracket_id),
          getBracketMatchups(matchupData.bracket_id),
        ]);
        setBracket(b.data?.bracket ?? b.data?.response ?? b.data);
        const msPayload = ms.data?.matchups ?? ms.data?.response ?? ms.data ?? [];
        setBracketMatchups(
          Array.isArray(msPayload)
            ? msPayload
            : Array.isArray(msPayload.matchups)
            ? msPayload.matchups
            : [],
        );
      }
    } else {
      setBracket(null);
      setBracketMatchups([]);
    }
  }, [uid, id, viewerId, promptUpgrade]);

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
      document.title = `${matchup.title} | Matchup`;
      return () => { document.title = 'Matchup'; };
    }
  }, [matchup?.title]);

  useEffect(() => {
    // Guard against stale state: a fast route change away from the
    // page (or a re-mount mid-fetch) shouldn't apply this fetch's
    // result to the now-different page. Without the cancelled flag,
    // a slow getCurrentUser response could set the viewer state
    // AFTER the page navigated, contaminating the owner gate on the
    // next mount.
    let cancelled = false;
    async function loadMe() {
      try {
        const res = await getCurrentUser();
        if (cancelled) return;
        setCurrentUser(res.data?.user ?? res.data?.response ?? res.data);
      } catch (err) {
        if (cancelled) return;
        console.warn("Unable to load current user", err);
      }
    }
    loadMe();
    return () => { cancelled = true; };
  }, []);

  /* ------------------------------------------------------------------ */
  /* TIMER + EXPIRATION */
  /* ------------------------------------------------------------------ */

  const matchupRound = matchup?.round ?? matchup?.Round ?? null;
  const isBracketMatchup = Boolean(matchup?.bracket_id);

  // "Match X of Y · Round N" progress hint, shown above the contender
  // cards on bracket matchups. Sort by seed (then id as tiebreak) to
  // match BracketView's ordering, so a viewer who sees "Match 2 of 4"
  // here would see this matchup in the second slot of the bracket
  // visualization. Renders nothing when bracket data hasn't loaded yet
  // or when the current matchup isn't part of a bracket.
  const matchProgress = useMemo(() => {
    if (!isBracketMatchup || !bracketMatchups.length || matchupRound == null) {
      return null;
    }
    const sameRound = bracketMatchups
      .filter((m) => Number(m.round ?? m.Round ?? 0) === Number(matchupRound))
      .sort((a, b) => {
        const sa = Number(a.seed ?? a.Seed ?? 0);
        const sb = Number(b.seed ?? b.Seed ?? 0);
        if (sa && sb && sa !== sb) return sa - sb;
        return String(a.id).localeCompare(String(b.id));
      });
    const idx = sameRound.findIndex(
      (m) => String(m.id) === String(matchup?.id ?? id),
    );
    if (idx === -1) return null;
    // Special-case the championship match: when there's only a single
    // match in the highest round, "Final" reads better than "Round 2".
    const allRounds = bracketMatchups
      .map((m) => Number(m.round ?? m.Round ?? 0))
      .filter((r) => r > 0);
    const maxRound = allRounds.length ? Math.max(...allRounds) : 0;
    const isFinal =
      sameRound.length === 1 && Number(matchupRound) === maxRound;
    return {
      position: idx + 1,
      total: sameRound.length,
      round: Number(matchupRound),
      label: isFinal ? "Final" : `Round ${matchupRound}`,
    };
  }, [bracketMatchups, isBracketMatchup, matchupRound, matchup?.id, id]);

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

  // Strict-string equality on both sides. Earlier truthy-and-equality
  // could in theory short-circuit through coercion edge cases (e.g.,
  // both sides numeric 0, or both undefined after a fast route change
  // with stale state). Owners reported seeing their Owner tray on
  // someone else's matchup; the hypothesis is one of (a) stale
  // currentUser from a prior viewer, (b) a matchup payload missing
  // author_id. The stricter check below makes both impossible: both
  // sides must be non-empty strings AND match.
  const isOwner =
    typeof currentUser?.id === 'string' &&
    currentUser.id.length > 0 &&
    typeof matchup?.author_id === 'string' &&
    matchup.author_id.length > 0 &&
    currentUser.id === matchup.author_id;

  // Dev-only telemetry: when the viewer is identified but the matchup
  // response lacks an author_id, log it once per mount so the next QA
  // repro of the owner-leak bug surfaces a concrete network trace.
  useEffect(() => {
    if (process.env.NODE_ENV === 'production') return;
    if (!matchup) return;
    if (currentUser?.id && !matchup.author_id) {
      // eslint-disable-next-line no-console
      console.warn(
        '[MatchupPage] author_id missing on matchup response — owner gate may misbehave',
        { matchupId: matchup.id, viewer: currentUser.id },
      );
    }
  }, [matchup, currentUser?.id]);

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
  const isReady = matchupStatus === "completed";
  // Strict owner-only — admins do NOT see Ready up on someone else's
  // matchup. Per UX call: owner controls are part of the author's
  // experience, not a generic moderator surface. Admin moderation,
  // if ever needed, belongs in a dedicated admin view, not inline on
  // the public matchup page. Backend authorization is the defensive
  // backstop; this gate keeps the chrome itself off-screen so a
  // non-owner viewer never sees a button they aren't supposed to use.
  const canReadyUp = isOwner && (isOpenStatus || isReady);
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

  // Strict owner-only — see canReadyUp note. Admins no longer see
  // Override winner on someone else's bracket matchup.
  const canOverrideWinner =
    isBracketMatchup &&
    isOwner &&
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

  // Reconcile the URL with the stored nav stack on every matchup
  // load. Three cases:
  //   1. current id === history[cursor]  → in-flow nav, no change.
  //   2. current id is elsewhere in the stack → cursor moved by
  //      browser back/forward (or a deep-link to a known entry);
  //      slide cursor to that index.
  //   3. current id not in history at all → fresh entry from /home or
  //      an external link. Reset history to a single-entry stack
  //      so the new matchup becomes the new origin.
  // This makes browser back/forward, deep-links, and "click a fresh
  // matchup from the feed" all work without explicit detection.
  useEffect(() => {
    if (!matchup?.id) return;
    const currentId = String(matchup.id);
    const slug = String(
      matchup?.author?.username ?? matchup?.author_id ?? uid ?? 'unknown',
    );
    const stored = readHistory();
    const idx = stored.history.findIndex((h) => h.id === currentId);
    let next;
    if (idx >= 0) {
      next = { history: stored.history, cursor: idx };
    } else {
      next = { history: [{ id: currentId, slug }], cursor: 0 };
    }
    writeHistory(next);
    setNavState(next);
    setNavPending(false);
  }, [matchup?.id, matchup?.author?.username, matchup?.author_id, uid]);

  // handleNext — Twitter-video swipe-up. If we're mid-stack, re-walk
  // the existing forward path (browser-style); only at the end do we
  // fetch fresh content. Filter excludes every id in history so the
  // pool can never serve us back a matchup we've already seen — fixes
  // the ping-pong bug where a small public-matchup pool repeatedly
  // bounced the viewer between two cards.
  const handleNext = async () => {
    if (navPending) return;
    setNavPending(true);
    try {
      // Mid-stack: browser-style replay forward.
      if (navState.cursor < navState.history.length - 1) {
        const fwd = { ...navState, cursor: navState.cursor + 1 };
        writeHistory(fwd);
        setNavState(fwd);
        const target = fwd.history[fwd.cursor];
        track('matchup_skipped', {
          matchup_id: matchup.id,
          surface: 'detail-next',
          forward_replay: true,
          from_pick: Boolean(votedItemId),
        });
        navigate(`/users/${target.slug}/matchup/${target.id}`);
        return;
      }

      // End of stack: fire skip + fetch a fresh pool in parallel.
      const [, listRes] = await Promise.allSettled([
        skipMatchup(matchup.id),
        getMatchups(1, 50),
      ]);

      let pool = [];
      if (listRes.status === 'fulfilled') {
        const payload = listRes.value?.data?.matchups
          ?? listRes.value?.data?.response
          ?? listRes.value?.data
          ?? {};
        pool = Array.isArray(payload)
          ? payload
          : Array.isArray(payload.matchups) ? payload.matchups : [];
      }
      const seen = new Set(navState.history.map((h) => h.id));
      seen.add(String(matchup.id));
      // Bracket-child matchups are now eligible — research feedback on
      // the home feed asked for bracket matchups (not whole brackets)
      // to be browseable from the swipe stream. The matchup detail
      // page already handles `bracket_id` set: it renders the parent
      // breadcrumb + "Match X of Y · Round N" chip and falls through
      // to the same vote/comment UI. Whole brackets are never in the
      // ListMatchups response (they live in the popular_brackets feed),
      // so we don't need an explicit "skip bracket roots" filter.
      //
      // Cycling-through-N bug fix: with the previous `!m.bracket_id`
      // filter, dev DBs that had 3 standalone matchups + N bracket
      // children produced a tiny pool that exhausted in 3 clicks and
      // bounced the viewer back to /home. Including bracket children
      // grows the pool dramatically.
      const candidates = pool.filter((m) =>
        !seen.has(String(m.id)) &&
        m.status !== 'completed'
      );

      track('matchup_skipped', {
        matchup_id: matchup.id,
        surface: 'detail-next',
        forward_replay: false,
        from_pick: Boolean(votedItemId),
        pool_size: candidates.length,
      });

      if (candidates.length === 0) {
        // Pool exhausted — viewer has seen everything we can show.
        // Falling back to /home rather than ping-ponging.
        navigate('/home');
        return;
      }

      const pick = candidates[Math.floor(Math.random() * candidates.length)];
      const slug = String(
        pick?.author?.username ?? pick?.author_id ?? 'unknown',
      );
      const appended = {
        history: [...navState.history, { id: String(pick.id), slug }],
        cursor: navState.history.length,
      };
      writeHistory(appended);
      setNavState(appended);
      navigate(`/users/${slug}/matchup/${pick.id}`);
    } catch (err) {
      // Promise.allSettled never rejects, so this is purely defensive.
      console.warn('next-matchup failed:', err);
      navigate('/home');
    }
  };

  // handlePrevious — Twitter-video swipe-down. Walks back through the
  // stack; the JSX disables the button at cursor === 0 so this branch
  // never fires from the UI, but the guard stays for safety.
  const handlePrevious = () => {
    if (navPending) return;
    if (navState.cursor <= 0) return;
    const back = { ...navState, cursor: navState.cursor - 1 };
    writeHistory(back);
    setNavState(back);
    const target = back.history[back.cursor];
    navigate(`/users/${target.slug}/matchup/${target.id}`);
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
          // Clear any earlier "matchup is tied" / "could not update"
          // banner — it's stale now that the winner is locked.
          setError(null);
        } catch (err) {
          console.error('overrideMatchupWinner failed', err);
          const serverMsg =
            err?.response?.data?.message ||
            err?.response?.data?.error ||
            err?.message;
          setError(serverMsg
            ? `Failed to select winner: ${serverMsg}`
            : "Failed to select winner.");
        }
      },
    });
  };

  const handleReadyUp = () => {
    if (readyPending) return;
    // Surface the blocker reason instead of silently no-op'ing on a
    // disabled click. Owners reported "Ready up doesn't do anything" —
    // the disabled state was correct (tied vote needs a manual winner
    // first) but unexplained, so the click felt broken. Route the
    // click through this guard with a friendly error and the user
    // immediately sees what to do.
    if (canReadyUp && !isReady && requiresManualWinner) {
      setError("Pick a winner first — votes are tied. Use Override winner to choose.");
      return;
    }
    if (!readyEnabled) return;
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
          setError(null);
        } catch (err) {
          // Surface the actual server error rather than a generic
          // fallback. Owners reported "Ready up does nothing" on prod
          // because the backend was returning "matchup is tied" but
          // the UI hid that behind "Could not update matchup readiness."
          // — they had no clue what to do. Connect-RPC error messages
          // arrive as either err.response.data.message (HTTP body) or
          // err.message (after Connect wraps it). Prefer the first.
          console.error('completeMatchup failed', err);
          const serverMsg =
            err?.response?.data?.message ||
            err?.response?.data?.error ||
            err?.message;
          setError(serverMsg
            ? `Could not update matchup readiness: ${serverMsg}`
            : "Could not update matchup readiness.");
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
          setError(null);
        } catch (err) {
          console.error('activateMatchup failed', err);
          const serverMsg =
            err?.response?.data?.message ||
            err?.response?.data?.error ||
            err?.message;
          setError(serverMsg
            ? `Unable to activate matchup: ${serverMsg}`
            : "Unable to activate matchup.");
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

        {/* Twitter-style hero. Owner avatar on the left, byline + title
            + description + tags + countdown on the right. State (active
            / voting-closed / completed / draft) lives in a top-right
            corner pill so it doesn't compete with the title. Same
            structural pattern as the bracket-detail hero. */}
        {(() => {
          const ownerUsername = matchup?.author?.username || matchup?.author_username;
          const ownerId = matchup?.author?.id || matchup?.author_id || matchup?.authorId;
          const displayName = matchup?.author?.display_name || ownerUsername || authorName;
          const profileSlug = ownerUsername || ownerId;
          // State for the corner pill. Order matters: a draft beats
          // anything else; an expired matchup outranks "active" since
          // voting is actually closed; "completed" wins over generic
          // status fallthrough.
          let pillState = "unknown";
          let pillLabel = "UNKNOWN";
          if (matchupStatus === "draft") {
            pillState = "draft"; pillLabel = "DRAFT";
          } else if (matchupStatus === "completed") {
            pillState = "completed"; pillLabel = "COMPLETED";
          } else if (matchupExpired) {
            pillState = "closed"; pillLabel = "VOTING CLOSED";
          } else if (isOpenStatus) {
            pillState = "active"; pillLabel = "ACTIVE";
          } else if (matchupStatus) {
            pillState = String(matchupStatus).toLowerCase();
            pillLabel = String(matchupStatus).toUpperCase();
          }
          return (
            <section className="matchup-hero" aria-label="Matchup overview">
              <span
                className="matchup-hero__status"
                data-state={pillState}
                role="status"
              >
                <span className="matchup-hero__status-dot" aria-hidden="true" />
                <span className="matchup-hero__status-label">{pillLabel}</span>
              </span>

              {matchup.image_url && (
                <div className="matchup-hero__media">
                  <img src={matchup.image_url} alt={matchup.title} decoding="async" />
                </div>
              )}

              <div className="matchup-hero__body">
                {profileSlug && ownerId && (
                  <Link
                    to={`/users/${profileSlug}`}
                    className="matchup-hero__avatar"
                    aria-label={`${displayName || "Owner"} profile`}
                  >
                    <ProfilePic userId={ownerId} size={80} />
                  </Link>
                )}

                <div className="matchup-hero__text">
                  <header className="matchup-overline">
                    {ownerUsername ? (
                      <Link
                        to={`/users/${profileSlug}`}
                        className="matchup-byline-name"
                      >
                        {displayName}
                      </Link>
                    ) : (
                      <span className="matchup-byline-name">{displayName || "Unknown"}</span>
                    )}
                    <span className="matchup-byline-meta">
                      {ownerUsername && (
                        <Link
                          to={`/users/${profileSlug}`}
                          className="matchup-byline-handle"
                        >
                          @{ownerUsername}
                        </Link>
                      )}
                      {matchup?.created_at && (
                        <>
                          {ownerUsername && <span aria-hidden="true"> · </span>}
                          <time
                            className="matchup-byline-time"
                            dateTime={matchup.created_at}
                            title={new Date(matchup.created_at).toLocaleString()}
                          >
                            {timeAgo}
                          </time>
                        </>
                      )}
                      {roundLabel && (
                        <>
                          <span aria-hidden="true"> · </span>
                          <span className="matchup-byline-round">{roundLabel}</span>
                        </>
                      )}
                    </span>
                  </header>

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

                  {/* Inline status row keeps the live countdown for
                      active timer-driven matchups. The "Voting closed"
                      and generic "Status: X" inline pills moved to the
                      corner status pill above, so this row only carries
                      forward-looking info now. */}
                  {matchupEndsAt && !matchupExpired && (
                    <div className="matchup-status-row">
                      <div className="matchup-status-pill matchup-status-pill--timer">
                        ⏳ <strong>{countdown.formatted}</strong>
                      </div>
                    </div>
                  )}

                  {isTieAfterExpiryInActiveRound && (
                    <div className="matchup-status-banner matchup-status-banner--warning">
                      ⚠️ Voting ended in a tie. The owner must choose a winner before the round can advance.
                    </div>
                  )}
                </div>
              </div>
            </section>
          );
        })()}

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
          {/* Bracket progress chip — "Match X of Y · Round N" (or
              "Final" on the championship match). Surfaces sequence
              context for users who may have landed mid-bracket. Pure
              client-side computation against the loaded
              bracketMatchups list — no proto/RPC change. */}
          {matchProgress && (
            <div
              className="matchup-progress-chip"
              role="status"
              aria-live="polite"
              aria-label={`Match ${matchProgress.position} of ${matchProgress.total}, ${matchProgress.label}`}
            >
              <span className="matchup-progress-chip__primary">
                Match {matchProgress.position} of {matchProgress.total}
              </span>
              <span className="matchup-progress-chip__sep" aria-hidden="true">·</span>
              <span className="matchup-progress-chip__round">
                {matchProgress.label}
              </span>
            </div>
          )}
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
          {/* Pair-row contender layout. Items are chunked into rows of
              two; each row is its own 1fr auto 1fr grid with a centered
              VS divider in the middle slot. Odd-count tail rows (3, 5, 7
              items) get a single-card row with no VS — full-width via the
              --single modifier.

              Earlier versions tried two simpler shapes and both were
              wrong:
                - One flat grid for all children: 4 items + 1 divider =
                  5 children flowing into a 3-col grid → row 2 wraps with
                  the right column empty.
                - Vertical stack for items.length !== 2: the 2x2 layout
                  is what users expect when they create a 4-way poll;
                  stacking lost the bracket-style visual.
              Chunked rows handle 1, 2, 3, 4, 5+ cleanly without any
              special-cases. Each row mirrors one matchup pairing.
              On <720px every row collapses to a single column (see
              MatchupPage.css). */}
          <div className="matchup-items">
            {(() => {
              const pairs = [];
              for (let i = 0; i < items.length; i += 2) {
                pairs.push(items.slice(i, i + 2));
              }
              return pairs.map((pair, pairIdx) => (
                <div
                  key={pairIdx}
                  className={`matchup-pair-row${
                    pair.length === 1 ? " matchup-pair-row--single" : ""
                  }`}
                >
                  {pair.map((item, idx) => (
                    <React.Fragment key={item.id}>
                      {idx === 1 && (
                        <div
                          className="matchup-vs-divider"
                          aria-hidden="true"
                        >
                          VS
                        </div>
                      )}
                      <MatchupItem
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
                    </React.Fragment>
                  ))}
                </div>
              ));
            })()}
          </div>

          {/* Twitter-video stream nav: "← Previous" + "Next matchup →".
              Next walks forward through the in-session history (or
              fetches a fresh public matchup at the end of the stack
              and appends it). Previous walks back through history;
              disabled at cursor === 0 (the origin matchup the viewer
              entered the flow on). The disabled state lights up with
              a subtle gray so it reads as "you're at the start" —
              v1 doesn't auto-exit to the timeline at origin like
              Twitter does; the browser back button covers that.
              Hidden when the matchup is resolved (winner declared) —
              voting is closed and there's nothing to swipe past. */}
          {displayWinnerId === null && (
            <div className="matchup-nav-row">
              <button
                type="button"
                className="matchup-nav-btn matchup-nav-btn--prev"
                onClick={handlePrevious}
                disabled={navPending || navState.cursor <= 0}
                aria-label={
                  navState.cursor <= 0
                    ? "Previous matchup (at start)"
                    : "Go to previous matchup"
                }
              >
                ← Previous
              </button>
              <button
                type="button"
                className="matchup-nav-btn matchup-nav-btn--next"
                onClick={handleNext}
                disabled={navPending}
                aria-label="Go to next matchup"
              >
                {navPending ? "Loading…" : "Next matchup →"}
              </button>
            </div>
          )}
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
          {/* Report button removed at user request. The ReportService
              backend handler stays in place — re-enabling is a one-line
              JSX restore + the FiFlag import below if it gets removed. */}
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
                  /* Only disable during the in-flight API call. The
                     blocked-by-tied-vote case routes through
                     handleReadyUp's guard so the click surfaces a
                     real explanation instead of a silent no-op. */
                  disabled={readyPending}
                  title={
                    !isReady && requiresManualWinner
                      ? "Tied vote — pick a winner first via Override winner"
                      : "Closes voting & reveals winner"
                  }
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
