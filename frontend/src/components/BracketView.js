import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { incrementMatchupItemVotes } from "../services/api";
import "../styles/BracketView.css";

export default function BracketView({
  matchups = [],
  bracket,
  champion,
}) {
  const [localMatchups, setLocalMatchups] = useState(matchups);
  const [votePendingId, setVotePendingId] = useState(null);
  const [activeRound, setActiveRound] = useState(null);
  const [hoveredMatchId, setHoveredMatchId] = useState(null);
  const roundRefs = useRef({});
  const navigate = useNavigate();

  useEffect(() => {
    setLocalMatchups(matchups);
  }, [matchups]);


  const rounds = useMemo(
    () =>
      localMatchups.reduce((acc, matchup) => {
        const r = Number(matchup.round || 1);
        acc[r] = acc[r] || [];
        acc[r].push(matchup);
        return acc;
      }, {}),
    [localMatchups],
  );

  const orderedRounds = Object.keys(rounds)
    .map(Number)
    .sort((a, b) => a - b);

  const finalRound = orderedRounds.length ? Math.max(...orderedRounds) : 0;

  useEffect(() => {
    if (!activeRound && orderedRounds.length) {
      setActiveRound(orderedRounds[0]);
    }
  }, [activeRound, orderedRounds]);

  const layoutConfig = useMemo(
    () => ({
      columnWidth: 260,
      columnGap: 80,
      matchHeight: 112,
      matchGap: 24,
      labelOffset: 32,
    }),
    [],
  );

  const roundLayouts = useMemo(() => {
    const round1Count = rounds[orderedRounds[0]]?.length || 0;
    const baseSpacing = layoutConfig.matchHeight + layoutConfig.matchGap;
    const canvasHeight = Math.max(round1Count, 1) * baseSpacing + layoutConfig.labelOffset;

    const layouts = orderedRounds.map((round, roundIndex) => {
      const matches = [...(rounds[round] || [])].sort((a, b) => {
        const seedA = Number(a.seed ?? a.Seed ?? 0);
        const seedB = Number(b.seed ?? b.Seed ?? 0);
        if (seedA && seedB && seedA !== seedB) return seedA - seedB;
        return String(a.id).localeCompare(String(b.id));
      });
      const spacing = baseSpacing * Math.pow(2, roundIndex);
      const positions = matches.map((_, index) => {
        const center = spacing / 2 + index * spacing;
        return layoutConfig.labelOffset + center - layoutConfig.matchHeight / 2;
      });
      return {
        round,
        roundIndex,
        matches,
        positions,
        spacing,
        left: roundIndex * (layoutConfig.columnWidth + layoutConfig.columnGap),
      };
    });

    return {
      layouts,
      canvasHeight,
      canvasWidth:
        layouts.length * layoutConfig.columnWidth +
        Math.max(layouts.length - 1, 0) * layoutConfig.columnGap,
      baseSpacing,
    };
  }, [layoutConfig, orderedRounds, rounds]);

  const connectors = useMemo(() => {
    const paths = [];
    const { layouts } = roundLayouts;
    layouts.forEach((layout, index) => {
      const nextLayout = layouts[index + 1];
      if (!nextLayout) return;

      layout.matches.forEach((matchup, matchIndex) => {
        const startX = layout.left + layoutConfig.columnWidth;
        const startY = layout.positions[matchIndex] + layoutConfig.matchHeight / 2;
        const nextIndex = Math.floor(matchIndex / 2);
        const endX = nextLayout.left;
        const endY =
          nextLayout.positions[nextIndex] + layoutConfig.matchHeight / 2;
        const midX = startX + layoutConfig.columnGap / 2;
        const winnerId =
          matchup.winner_item_id ??
          matchup.winnerItemId ??
          matchup.winner_item?.id ??
          matchup.winnerItem?.id ??
          null;
        const isActive = Boolean(winnerId);

        paths.push({
          id: `${matchup.id}-to-${nextLayout.round}-${nextIndex}`,
          d: `M ${startX} ${startY} H ${midX} V ${endY} H ${endX}`,
          isActive,
          matchupId: matchup.id,
        });
      });
    });

    return paths;
  }, [layoutConfig, roundLayouts]);

  const handleRoundTabClick = (round, scrollRound = round) => {
    setActiveRound(round);
    const node = roundRefs.current[scrollRound];
    if (node?.scrollIntoView) {
      node.scrollIntoView({ behavior: "smooth", inline: "center" });
    }
  };

  const handleVote = async (matchupId, itemId) => {
    if (votePendingId === itemId) return;
    if (!itemId) return;

    try {
      setVotePendingId(itemId);
      const res = await incrementMatchupItemVotes(itemId);
      const payload = res?.data?.response ?? res?.data ?? {};
      const updatedVotes = payload?.votes ?? payload?.Votes;

      setLocalMatchups((prev) =>
        prev.map((matchup) => {
          if (matchup.id !== matchupId) return matchup;
          const updatedItems = (matchup.items || []).map((item) => {
            if (item.id !== itemId) return item;
            return {
              ...item,
              votes:
                typeof updatedVotes === "number"
                  ? updatedVotes
                  : Number(item?.votes ?? 0) + 1,
            };
          });
          return { ...matchup, items: updatedItems };
        }),
      );
    } catch (err) {
      console.error("Unable to register vote", err);
    } finally {
      setVotePendingId(null);
    }
  };

  const renderRow = (matchup, item, totalVotes, leadingId) => {
    const winnerId =
      matchup.winner_item_id ??
      matchup.winnerItemId ??
      matchup.winner_item?.id ??
      matchup.winnerItem?.id ??
      null;
    const isWinner = winnerId && String(winnerId) === String(item.id);
    const isLeading = !winnerId && leadingId && String(leadingId) === String(item.id);
    const votes = Number(item?.votes ?? item?.Votes ?? 0);
    const percent =
      totalVotes > 0 ? Math.round((votes / totalVotes) * 100) : 0;
    const isActiveRound =
      bracket?.status === "active" &&
      Number(matchup.round ?? matchup.Round ?? 0) ===
        Number(bracket.current_round ?? bracket.currentRound ?? 0);
    const isVotingLocked = bracket?.status !== "active" || !isActiveRound;
    const isPending = votePendingId === item.id;
    const canVote = !item?.placeholder && !isVotingLocked && !isPending;

    const rowClasses = [
      "bracket-row",
      item?.placeholder ? "is-placeholder" : "",
      isWinner ? "is-winner" : "",
      !isWinner && winnerId ? "is-loser" : "",
      isLeading ? "is-leading" : "",
      isVotingLocked ? "is-locked" : "",
    ]
      .filter(Boolean)
      .join(" ");

    return (
      <button
        type="button"
        key={item.id}
        className={rowClasses}
        onClick={(event) => {
          event.stopPropagation();
          if (canVote) {
            handleVote(matchup.id, item.id);
          }
        }}
        disabled={!canVote}
      >
        <span className="bracket-row-name">
          {item.placeholder ? "Awaiting winner" : item.item}
        </span>
        <span className="bracket-row-votes">{percent}%</span>
        <div className="bracket-row-bar">
          <motion.span
            className="bracket-row-bar-fill"
            animate={{ width: `${percent}%` }}
            transition={{ duration: 0.3 }}
          />
        </div>
      </button>
    );
  };

  return (
    <div className="bracket-wrapper">
      <AnimatePresence>
        {champion && (
          <motion.div
            className="bracket-champion"
            initial={{ opacity: 0, scale: 0.94 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.96 }}
            transition={{ duration: 0.3 }}
          >
            <div className="champion-label">🏆 Champion</div>
            <div className="champion-name">{champion.item.item}</div>
          </motion.div>
        )}
      </AnimatePresence>

      {orderedRounds.length === 0 ? (
        <div className="bracket-empty-card">
          No matchups yet. Seeds will appear here once the bracket is ready.
        </div>
      ) : (
        <>
          <div className="bracket-round-tabs">
            {orderedRounds.map((round) => (
              <button
                key={`tab-${round}`}
                type="button"
                className={`bracket-round-tab ${
                  round === activeRound ? "is-active" : ""
                }`}
                onClick={() => handleRoundTabClick(round)}
              >
                Round {round}
              </button>
            ))}
            <button
              type="button"
              className={`bracket-round-tab ${
                activeRound === finalRound + 1 ? "is-active" : ""
              }`}
              onClick={() => handleRoundTabClick(finalRound + 1, finalRound)}
            >
              Champion
            </button>
          </div>

          <div
            className="bracket-canvas"
            style={{
              height: roundLayouts.canvasHeight,
              minWidth: roundLayouts.canvasWidth,
            }}
          >
            <svg
              className="bracket-connectors"
              width={roundLayouts.canvasWidth}
              height={roundLayouts.canvasHeight}
            >
              {connectors.map((connector) => (
                <path
                  key={connector.id}
                  d={connector.d}
                  className={`bracket-connector ${
                    connector.isActive ||
                    (hoveredMatchId &&
                      String(connector.matchupId) === String(hoveredMatchId))
                      ? "is-active"
                      : ""
                  }`}
                />
              ))}
            </svg>

            {roundLayouts.layouts.map((layout) => {
              const isCompleted =
                bracket.status === "completed" ||
                layout.round < bracket.current_round;
              const isFinal = layout.round === finalRound;
              const isActiveColumn =
                activeRound === layout.round ||
                (activeRound === finalRound + 1 && layout.round === finalRound);

              return (
                <div
                  key={`round-${layout.round}`}
                  className={`bracket-column ${
                    isCompleted ? "completed" : ""
                  } ${isFinal ? "final-round" : ""} ${
                    isActiveColumn ? "is-active" : ""
                  }`}
                  style={{
                    left: layout.left,
                    width: layoutConfig.columnWidth,
                    height: roundLayouts.canvasHeight,
                  }}
                  ref={(node) => {
                    roundRefs.current[layout.round] = node;
                  }}
                >
                  <p className="bracket-column-label">Round {layout.round}</p>
                  {layout.matches.map((matchup, index) => {
                    const items = Array.isArray(matchup.items)
                      ? matchup.items
                      : [];
                    const displayItems =
                      items.length > 0
                        ? items
                        : [
                            {
                              id: `${matchup.id}-placeholder-a`,
                              item: "Awaiting winner",
                              votes: 0,
                              placeholder: true,
                            },
                            {
                              id: `${matchup.id}-placeholder-b`,
                              item: "Awaiting winner",
                              votes: 0,
                              placeholder: true,
                            },
                          ];
                    const totalVotes = items.reduce(
                      (sum, item) => sum + Number(item?.votes ?? 0),
                      0,
                    );
                    const leadingId = items.reduce((leading, item) => {
                      if (!leading) return item.id;
                      const leadingVotes =
                        items.find((i) => i.id === leading)?.votes ?? 0;
                      const currentVotes = item?.votes ?? 0;
                      if (currentVotes > leadingVotes) {
                        return item.id;
                      }
                      if (currentVotes === leadingVotes) {
                        return null;
                      }
                      return leading;
                    }, null);

                    return (
                      <motion.div
                        key={matchup.id}
                        className="bracket-match bracket-match--clickable"
                        style={{
                          top: layout.positions[index],
                          height: layoutConfig.matchHeight,
                        }}
                        onMouseEnter={() => setHoveredMatchId(matchup.id)}
                        onMouseLeave={() => setHoveredMatchId(null)}
                        onClick={() => {
                          const authorSlug =
                            matchup?.author?.username ||
                            bracket?.author?.username ||
                            bracket?.author_id ||
                            bracket?.authorId ||
                            bracket?.author?.id ||
                            null;
                          if (!authorSlug) return;
                          navigate(`/users/${authorSlug}/matchup/${matchup.id}`);
                        }}
                        whileHover={{ scale: 1.01 }}
                        initial={{ opacity: 0, y: 12 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.25 }}
                      >
                        <div className="bracket-match-header">
                          <span>
                            Match {index + 1}
                          </span>
                          {layout.round === bracket.current_round &&
                            bracket.status === "active" && (
                              <span className="bracket-match-live">Live</span>
                            )}
                          <span className="bracket-match-link">View</span>
                        </div>
                        <div className="bracket-match-rows">
                          {displayItems.map((item) =>
                            renderRow(matchup, item, totalVotes, leadingId),
                          )}
                        </div>
                      </motion.div>
                    );
                  })}
                </div>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
}
