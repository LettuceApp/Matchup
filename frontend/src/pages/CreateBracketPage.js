import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import { createBracket, getCurrentUser } from "../services/api";
import "../styles/CreateBracketPage.css";

export default function CreateBracketPage() {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [size, setSize] = useState(8);

  const [advanceMode, setAdvanceMode] = useState("manual");
  const [durationMinutes, setDurationMinutes] = useState(1440);
  const [durationSeconds, setDurationSeconds] = useState(0);
  const [entries, setEntries] = useState(() => Array.from({ length: 8 }, () => ""));

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [currentUser, setCurrentUser] = useState(null);

  const navigate = useNavigate();

  useEffect(() => {
    const loadMe = async () => {
      try {
        const res = await getCurrentUser();
        setCurrentUser(res.data.response ?? res.data ?? null);
      } catch (err) {
        console.warn("Unable to load current user", err);
        setError("We couldn't verify your account right now.");
      }
    };
    loadMe();
  }, []);

  useEffect(() => {
    setEntries((prev) => {
      const next = prev.slice(0, size);
      while (next.length < size) {
        next.push("");
      }
      return next;
    });
  }, [size]);

  async function handleCreate(e) {
    e.preventDefault();
    if (!currentUser) {
      setError("Please sign in before creating a bracket.");
      return;
    }

    setLoading(true);
    setError(null);
    try {
      const payload = {
        title,
        description,
        size,
        advance_mode: advanceMode,
        entries: entries.map((entry) => entry.trim()),
      };
      if (advanceMode === "timer") {
        const totalSeconds = durationMinutes * 60 + durationSeconds;
        if (totalSeconds < 60 || totalSeconds > 86400) {
          setError("Round timer must be between 1 minute and 24 hours.");
          return;
        }
        payload.round_duration_seconds = totalSeconds;
      }

      const { data } = await createBracket(currentUser.id, payload);
      navigate(`/brackets/${data.response.id}`);
    } catch (err) {
      console.error("Failed to create bracket", err);
      setError("We couldn't create that bracket right now.");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="create-bracket-page">
      <NavigationBar />

      <main className="create-bracket-content">
        <section className="create-bracket-hero">
          <p className="create-bracket-overline">New bracket</p>
          <h1>Build the tournament arc for your community.</h1>
          <p>
            Pick the bracket size, set the pace, and let every matchup follow the same round
            timer.
          </p>
        </section>

        <section className="create-bracket-panel">
          <form onSubmit={handleCreate} className="create-bracket-form">
            <div className="create-bracket-field">
              <label htmlFor="bracket-title">Title</label>
              <input
                id="bracket-title"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                required
              />
            </div>

            <div className="create-bracket-field">
              <label htmlFor="bracket-description">Description</label>
              <textarea
                id="bracket-description"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                rows={4}
              />
            </div>

            <div className="create-bracket-field">
              <label htmlFor="bracket-size">Size</label>
              <select
                id="bracket-size"
                value={size}
                onChange={(e) => setSize(Number(e.target.value))}
              >
                {[4, 8, 16, 32, 64].map((n) => (
                  <option key={n} value={n}>
                    {n} Teams
                  </option>
                ))}
              </select>
            </div>

            <div className="create-bracket-field">
              <label htmlFor="bracket-advance-mode">Round advancement</label>
              <select
                id="bracket-advance-mode"
                value={advanceMode}
                onChange={(e) => setAdvanceMode(e.target.value)}
              >
                <option value="manual">Manual (owner advances rounds)</option>
                <option value="timer">Timed (auto-advance rounds)</option>
              </select>
            </div>

            {advanceMode === "timer" && (
              <div className="create-bracket-field">
                <label>Round duration</label>
                <div className="create-bracket-duration">
                  <div className="create-bracket-duration-field">
                    <input
                      type="number"
                      min={0}
                      max={1440}
                      value={durationMinutes}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setDurationMinutes(Number.isFinite(next) ? next : 0);
                      }}
                      required
                    />
                    <span>Minutes</span>
                  </div>
                  <div className="create-bracket-duration-field">
                    <input
                      type="number"
                      min={0}
                      max={59}
                      value={durationSeconds}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setDurationSeconds(Number.isFinite(next) ? next : 0);
                      }}
                      required
                    />
                    <span>Seconds</span>
                  </div>
                </div>
              </div>
            )}

            <div className="create-bracket-field">
              <label>Seeded contenders</label>
              <p className="create-bracket-hint">
                Add names for each seed. Seeds are preserved in the bracket (Seed 1 vs Seed {size}).
              </p>
              <div className="create-bracket-seeds">
                {entries.map((entry, index) => (
                  <div key={index} className="create-bracket-seed-row">
                    <span className="create-bracket-seed-label">Seed {index + 1}</span>
                    <input
                      type="text"
                      value={entry}
                      placeholder={`Name for seed ${index + 1}`}
                      onChange={(e) => {
                        const value = e.target.value;
                        setEntries((prev) =>
                          prev.map((seed, seedIndex) =>
                            seedIndex === index ? value : seed
                          )
                        );
                      }}
                    />
                  </div>
                ))}
              </div>
            </div>

            {error && <p className="create-bracket-error">{error}</p>}

            <div className="create-bracket-actions">
              <Button
                type="button"
                onClick={() => navigate(-1)}
                className="create-bracket-button create-bracket-button--ghost"
              >
                Go back
              </Button>
              <Button
                type="submit"
                disabled={loading}
                className="create-bracket-button"
              >
                {loading ? "Creating…" : "Create bracket"}
              </Button>
            </div>
          </form>
        </section>
      </main>
    </div>
  );
}
