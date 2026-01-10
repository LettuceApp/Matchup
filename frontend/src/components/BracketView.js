import MatchupCard from "./MatchupCard";
import "../styles/BracketView.css";

export default function BracketView({
  matchups = [],
  bracket,
  champion,
  likedMatchupIds = new Set(),
}) {
  const rounds = matchups.reduce((acc, matchup) => {
    const r = Number(matchup.round || 1);
    acc[r] = acc[r] || [];
    acc[r].push(matchup);
    return acc;
  }, {});

  const orderedRounds = Object.keys(rounds)
    .map(Number)
    .sort((a, b) => a - b);

  const finalRound = Math.max(...orderedRounds);

  return (
    <div className="bracket-wrapper">
      {champion && (
        <div className="bracket-champion">
          <div className="champion-label">🏆 Champion</div>
          <div className="champion-name">{champion.item.item}</div>
        </div>
      )}

      <div className="bracket-grid">
        {orderedRounds.map((round) => {
          const isCompleted =
            bracket.status === "completed" || round < bracket.current_round;

          const isFinal = round === finalRound;

          return (
            <div
              key={round}
              className={`bracket-column ${
                isCompleted ? "completed" : ""
              } ${isFinal ? "final-round" : ""}`}
            >
              <p className="bracket-column-label">Round {round}</p>

              <div className="bracket-column-list">
                {rounds[round].map((matchup) => {
                  const winnerId = matchup.winner_item_id;

                  return (
                    <MatchupCard
                      key={matchup.id}
                      matchup={matchup}
                      variant="bracket"
                      winnerItemId={winnerId}
                      isFinal={isFinal}
                      bracket={bracket}
                      likedMatchupIds={likedMatchupIds}
                    />
                  );
                })}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
