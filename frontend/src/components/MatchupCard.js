import { Link } from "react-router-dom";
import { useEffect, useMemo, useState } from "react";
import { motion } from "framer-motion";
import MatchupItem from "./MatchupItem";
import MatchupChart from "./MatchupChart";
import Comment from "./Comment";
import { getUserLikes, likeMatchup, unlikeMatchup } from "../services/api";

const MotionLink = motion(Link);

export default function MatchupCard({
  matchup,
  variant,
  placeholderLabels = [],
  ownerId,
  bracket,
  likedMatchupIds,
}) {
  const isBracket = variant === "bracket";
  const viewerId = useMemo(() => localStorage.getItem("userId"), []);
  const matchupOwnerId =
    ownerId ??
    matchup?.author_id ??
    matchup?.authorId ??
    matchup?.AuthorID ??
    matchup?.user_id ??
    matchup?.userId ??
    matchup?.UserID ??
    matchup?.owner_id ??
    matchup?.ownerId ??
    matchup?.OwnerID ??
    matchup?.author?.id ??
    matchup?.author?.ID ??
    matchup?.Author?.id ??
    matchup?.Author?.ID ??
    null;
  const normalizedOwnerId =
    matchupOwnerId !== undefined &&
    matchupOwnerId !== null &&
    matchupOwnerId !== ""
      ? String(matchupOwnerId)
      : null;
  const profileSlug = matchup?.author?.username || normalizedOwnerId;
  const items = Array.isArray(matchup.items) ? matchup.items : [];
  const roundNumber =
    typeof matchup.round === "number"
      ? matchup.round
      : Number.parseInt(matchup.round ?? "", 10);
  const roundLabel =
    Number.isFinite(roundNumber) && roundNumber > 0
      ? `Round ${roundNumber}`
      : null;
  const seedLabel =
    typeof matchup.seed === "number" ? `Seed ${matchup.seed}` : null;
  const placeholders =
    isBracket && placeholderLabels.length
      ? placeholderLabels.slice(0, Math.max(0, 2 - items.length))
      : [];
  const totalVotes = useMemo(
    () =>
      items.reduce(
        (sum, item) => sum + Number(item?.votes ?? item?.Votes ?? 0),
        0,
      ),
    [items],
  );
  const matchupStatus = matchup?.status ?? matchup?.Status ?? "active";
  const isOpenStatus = matchupStatus === "published" || matchupStatus === "active";
  const endsAtRaw = matchup?.end_time ?? matchup?.EndTime ?? matchup?.endTime ?? null;
  const endsAt = endsAtRaw ? new Date(endsAtRaw) : null;
  const isExpired = Boolean(endsAt && Date.now() >= endsAt.getTime());
  const isCompleted = matchupStatus === "completed";
  const matchupRound = Number(matchup?.round ?? matchup?.Round ?? 0);
  const bracketCurrentRound = Number(
    bracket?.current_round ?? bracket?.currentRound ?? 0,
  );
  const isActiveBracketRound =
    isBracket &&
    bracket?.status === "active" &&
    matchupRound === bracketCurrentRound;
  const interactionLocked =
    isCompleted ||
    isExpired ||
    (!isBracket && !isOpenStatus) ||
    (isBracket && !isActiveBracketRound);
  const canLike = Boolean(viewerId) && !interactionLocked;

  const [likesCount, setLikesCount] = useState(
    Number(matchup?.likes_count ?? matchup?.likesCount ?? 0),
  );
  const [isLiked, setIsLiked] = useState(
    Boolean(
      matchup?.is_liked ??
        matchup?.isLiked ??
        matchup?.liked_by_viewer ??
        matchup?.likedByViewer,
    ),
  );
  const [likePending, setLikePending] = useState(false);

  useEffect(() => {
    setLikesCount(Number(matchup?.likes_count ?? matchup?.likesCount ?? 0));
  }, [matchup?.likes_count, matchup?.likesCount]);

  useEffect(() => {
    if (!viewerId || !matchup?.id) return;
    if (likedMatchupIds instanceof Set) {
      setIsLiked(likedMatchupIds.has(String(matchup.id)));
      return;
    }

    let isMounted = true;
    getUserLikes(viewerId)
      .then((res) => {
        if (!isMounted) return;
        const likes = res.data?.response ?? res.data ?? [];
        const liked = Array.isArray(likes)
          ? likes.some(
              (like) => String(like.matchup_id) === String(matchup.id),
            )
          : false;
        setIsLiked(liked);
      })
      .catch((err) => {
        console.warn("Unable to load user likes", err);
      });

    return () => {
      isMounted = false;
    };
  }, [viewerId, matchup?.id, likedMatchupIds]);

  const handleLikeToggle = async (event) => {
    if (event) {
      event.preventDefault();
      event.stopPropagation();
    }
    if (!matchup?.id || likePending || !canLike) return;

    try {
      setLikePending(true);
      if (isLiked) {
        await unlikeMatchup(matchup.id);
        setIsLiked(false);
        setLikesCount((count) => Math.max(0, count - 1));
      } else {
        await likeMatchup(matchup.id);
        setIsLiked(true);
        setLikesCount((count) => count + 1);
      }
    } catch (err) {
      const message = err?.response?.data?.error ?? err?.message ?? "";
      if (typeof message === "string" && message.includes("already liked")) {
        setIsLiked(true);
      } else {
        console.error("Unable to update like", err);
      }
    } finally {
      setLikePending(false);
    }
  };

  const cardMotion = {
    layout: true,
    initial: { opacity: 0, y: 14 },
    animate: { opacity: 1, y: 0 },
    exit: { opacity: 0, y: -10 },
    transition: { duration: 0.25 },
  };

  const cardBody = (
    <>
      <h4 className="matchup-title">{matchup.title}</h4>

      {isBracket && (roundLabel || seedLabel) && (
        <div className="matchup-meta">
          {roundLabel && <span>{roundLabel}</span>}
          {seedLabel && <span>{seedLabel}</span>}
        </div>
      )}

      <div className="matchup-items">
        {items.map((item) => (
          <MatchupItem
            key={item.id}
            item={item}
            matchupId={matchup.id}
            disabled={false}
            isBracketMatchup={isBracket}
            isVotingLocked={interactionLocked}
            totalVotes={totalVotes}
            showVoteBar={isBracket}
          />
        ))}

        {placeholders.map((label, index) => (
          <div key={`placeholder-${index}`} className="matchup-placeholder">
            <span className="matchup-placeholder-label">{label}</span>
            <span className="matchup-placeholder-status">Awaiting winner</span>
          </div>
        ))}
      </div>

      {!isBracket && (
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
      )}

      {!isBracket && <MatchupChart matchup={matchup} />}
      {!isBracket && <Comment matchupId={matchup.id} />}
    </>
  );

  if (isBracket) {
    const bracketMatchupPath =
      profileSlug && matchup?.id
        ? `/users/${profileSlug}/matchup/${matchup.id}`
        : null;

    if (!bracketMatchupPath) {
      return (
        <motion.div
          className="matchup-card bracket"
          whileHover={{ y: -4 }}
          {...cardMotion}
        >
          {cardBody}
        </motion.div>
      );
    }

    return (
      <MotionLink
        to={bracketMatchupPath}
        className="matchup-card bracket matchup-card-link"
        whileHover={{ y: -4 }}
        {...cardMotion}
      >
        {cardBody}
      </MotionLink>
    );
  }

  return (
    <motion.div className="matchup-card" whileHover={{ y: -4 }} {...cardMotion}>
      {cardBody}
    </motion.div>
  );
}
