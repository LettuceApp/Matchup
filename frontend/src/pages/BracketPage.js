import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import BracketView from "../components/BracketView";
import {
  getBracket,
  getBracketMatchups,
  updateBracket,
  advanceBracket,
  getCurrentUser,
} from "../services/api";
import "../styles/BracketPage.css";

export default function BracketPage() {
  const { id } = useParams();
  const [bracket, setBracket] = useState(null);
  const [matchups, setMatchups] = useState([]);
  const [currentUser, setCurrentUser] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [confirm, setConfirm] = useState(false);

  useEffect(() => {
    async function loadData() {
      try {
        const [b, m, u] = await Promise.all([
          getBracket(id),
          getBracketMatchups(id),
          getCurrentUser(),
        ]);
        setBracket(b.data.response);
        setMatchups(m.data.response || []);
        setCurrentUser(u.data.response);
      } catch (err) {
        console.error(err);
        setError("We couldn't load this bracket right now.");
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, [id]);

  const isOwner = bracket && currentUser?.id === bracket.author_id;

  const handleActivate = async () => {
    try {
      await updateBracket(bracket.id, { status: "active" });
      setBracket({ ...bracket, status: "active" });
    } catch (err) {
      console.error(err);
      setError("Unable to activate bracket.");
    } finally {
      setConfirm(false);
    }
  };

  const handleAdvance = async () => {
    try {
      const res = await advanceBracket(bracket.id);
      setBracket({ ...bracket, current_round: res.data.round });
    } catch (err) {
      console.error(err);
      setError("Unable to advance round.");
    }
  };

  return (
    <div className="bracket-page">
      <NavigationBar />
      <main className="bracket-content">
        {loading && (
          <div className="bracket-status-card">Loading bracket…</div>
        )}

        {!loading && !bracket && (
          <div className="bracket-status-card bracket-status-card--error">
            Bracket not found.
          </div>
        )}

        {error && (
          <div className="bracket-status-card bracket-status-card--warning">
            {error}
          </div>
        )}

        {bracket && (
          <>
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
                      {bracket.status?.toUpperCase()}
                    </p>
                  </div>
                  <div>
                    <span className="bracket-meta-label">Current round</span>
                    <p className="bracket-meta-value">
                      {bracket.current_round || 1}
                    </p>
                  </div>
                </div>
              </div>

              {isOwner && (
                <div className="bracket-hero-actions">
                  {bracket.status === "draft" && (
                    <Button
                      onClick={() => setConfirm(true)}
                      className="bracket-button"
                    >
                      Activate bracket
                    </Button>
                  )}
                  {bracket.status === "active" && (
                    <Button
                      onClick={handleAdvance}
                      className="bracket-button"
                    >
                      Advance to next round
                    </Button>
                  )}
                </div>
              )}
            </section>

            <section className="bracket-section">
              <header className="bracket-section-header">
                <div>
                  <h2>Bracket matches</h2>
                  <p>
                    Follow each round as contenders progress toward the final.
                  </p>
                </div>
              </header>
              <BracketView matchups={matchups} />
            </section>
          </>
        )}
      </main>

      {confirm && (
        <div className="bracket-modal-overlay">
          <div className="bracket-modal">
            <h3>Activate bracket?</h3>
            <p>This will lock editing and allow advancing rounds.</p>
            <div className="bracket-modal-actions">
              <Button onClick={handleActivate} className="bracket-button">
                Activate
              </Button>
              <Button
                onClick={() => setConfirm(false)}
                className="bracket-button bracket-button--ghost"
              >
                Cancel
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
