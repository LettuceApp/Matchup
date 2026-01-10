import { useCallback, useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import BracketView from "../components/BracketView";
import {
  getBracketSummary,
  getCurrentUser,
  updateBracket,
  advanceBracket,
  deleteBracket,
  likeBracket,
  unlikeBracket,
} from "../services/api";
import "../styles/BracketPage.css";
import useCountdown from "../hooks/useCountdown";

export default function BracketPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const viewerId = Number(localStorage.getItem("userId") || 0);

  const [bracket, setBracket] = useState(null);
  const [matchups, setMatchups] = useState([]);
  const [champion, setChampion] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [currentUser, setCurrentUser] = useState(null);
  const [likesCount, setLikesCount] = useState(0);
  const [isLiked, setIsLiked] = useState(false);
  const [likePending, setLikePending] = useState(false);
  const [likedMatchupIds, setLikedMatchupIds] = useState(new Set());

  /* ------------------------------------------------------------------ */
  /* DATA LOADING */
  /* ------------------------------------------------------------------ */

  const loadBracket = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const summaryRes = await getBracketSummary(id, viewerId);
      const summary = summaryRes.data?.response ?? summaryRes.data ?? {};
      const bracketData = summary.bracket ?? null;
      const matchupsData = summary.matchups ?? [];
      const likedIds = Array.isArray(summary.liked_matchup_ids)
        ? summary.liked_matchup_ids.map((value) => Number(value))
        : [];

      setBracket(bracketData);
      setMatchups(Array.isArray(matchupsData) ? matchupsData : []);
      detectChampion(bracketData, matchupsData);
      setLikesCount(
        Number(bracketData?.likes_count ?? bracketData?.likesCount ?? 0),
      );
      setIsLiked(Boolean(summary.liked_bracket));
      setLikedMatchupIds(new Set(likedIds));
    } catch (err) {
      console.error("Failed to load bracket", err);
      setError("We could not load this bracket right now.");
      setBracket(null);
      setMatchups([]);
      setChampion(null);
    } finally {
      setLoading(false);
    }
  }, [id, viewerId]);

  useEffect(() => {
    loadBracket();
  }, [loadBracket]);

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
      setLikedMatchupIds(new Set());
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

  // When the round timer expires, Dagster will advance it.
  // We simply refresh after a short delay.
  useEffect(() => {
    if (
      roundCountdown.isExpired &&
      bracket?.status === "active"
    ) {
      const timeout = setTimeout(() => {
        loadBracket();
      }, 3000);

      return () => clearTimeout(timeout);
    }
  }, [roundCountdown.isExpired, bracket?.status, loadBracket]);

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
          <div className="bracket-status-card">Loading bracket…</div>
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
    Number(currentUser.id) === Number(bracketOwnerId);

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

  return (
    <div className="bracket-page">
      <NavigationBar />
      <main className="bracket-content">
        <section className="bracket-hero">
          <div className="bracket-hero-text">
            <p className="bracket-overline">Bracket overview</p>
            <h1>{bracket.title}</h1>
            <p className="bracket-description">
              {bracket.description || "No description provided yet."}
            </p>

            <div className="bracket-meta-row">
              <div>
                <span className="bracket-meta-label">Status</span>
                <p className="bracket-meta-value">
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
          </div>

          <div className="bracket-hero-actions">
            <div className="bracket-like-row">
              <button
                type="button"
                className={`bracket-like-button ${isLiked ? "is-liked" : ""}`}
                onClick={handleLikeToggle}
                disabled={!viewerId || likePending}
              >
                {likePending ? "Updating…" : isLiked ? "Liked" : "Like"}
              </button>
              <div className="bracket-like-indicator">
                <span className="bracket-like-count">{likesCount}</span>
                <span>Likes</span>
              </div>
            </div>

            {canEdit && (
              <>
                {bracket.status === "draft" && (
                  <Button onClick={handleActivate} className="bracket-button">
                    Activate
                  </Button>
                )}

                {bracket.status === "active" && (
                  <Button onClick={handleAdvance} className="bracket-button">
                    Advance to next round
                  </Button>
                )}

                <Button
                  onClick={handleDelete}
                  className="bracket-button bracket-button--danger"
                >
                  Delete
                </Button>
              </>
            )}
          </div>
        </section>

        <section className="bracket-section">
          <header className="bracket-section-header">
            <h2>Bracket matches</h2>
            <p>Follow each round as contenders progress.</p>
          </header>

          <BracketView
            matchups={matchups}
            bracket={bracket}
            champion={champion}
            likedMatchupIds={likedMatchupIds}
          />
        </section>
      </main>
    </div>
  );
}
