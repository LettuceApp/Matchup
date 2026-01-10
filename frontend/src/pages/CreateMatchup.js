import React, { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import { createMatchup } from "../services/api";
import "../styles/CreateMatchup.css";

const CreateMatchup = () => {
  const { userId } = useParams();
  const navigate = useNavigate();

  // Prefer the authenticated user id (localStorage), fallback to route param
  const authedUserId = Number(localStorage.getItem("userId") || userId);

  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [items, setItems] = useState([{ item: "" }, { item: "" }]);
  const [endMode, setEndMode] = useState("timer");
  const [durationMinutes, setDurationMinutes] = useState(60);
  const [durationSeconds, setDurationSeconds] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const maxItems = 5;

  const handleTitleChange = (e) => setTitle(e.target.value);
  const handleContentChange = (e) => setContent(e.target.value);

  const handleItemChange = (index, value) => {
    setItems((prev) => prev.map((it, i) => (i === index ? { ...it, item: value } : it)));
  };

  const addItem = () => {
    setItems((prev) => (prev.length < maxItems ? [...prev, { item: "" }] : prev));
  };

  const removeItem = (index) => {
    setItems((prev) => (prev.length > 1 ? prev.filter((_, i) => i !== index) : prev));
  };

  const goBack = () => navigate(-1);

  // Sanitize + validate
  const sanitizedItems = useMemo(
    () => items.map(({ item }) => ({ item: (item ?? "").trim() })),
    [items]
  );

  const filledItems = useMemo(
    () => sanitizedItems.filter(({ item }) => item.length > 0),
    [sanitizedItems]
  );

  const hasRequiredContenders = filledItems.length >= 2;

  // All contender inputs must be non-empty (no blank rows)
  const allInputsFilled = filledItems.length === items.length;

  const isCreateDisabled =
    isSubmitting ||
    title.trim().length === 0 ||
    content.trim().length === 0 ||
    !allInputsFilled ||
    !hasRequiredContenders ||
    !Number.isFinite(authedUserId) ||
    authedUserId <= 0;

  const handleSubmit = async (e) => {
    e.preventDefault();
    if (isCreateDisabled) return;

    setError(null);

    const matchupData = {
      title: title.trim(),
      content: content.trim(),
      items: sanitizedItems,
      status: "draft",
      end_mode: endMode,
    };
    if (endMode === "timer") {
      const totalSeconds = durationMinutes * 60 + durationSeconds;
      if (totalSeconds < 60 || totalSeconds > 86400) {
        setError("Timer must be between 1 minute and 24 hours.");
        return;
      }
      matchupData.duration_seconds = totalSeconds;
    }

    try {
      setIsSubmitting(true);
      setError(null);

      const response = await createMatchup(authedUserId, matchupData);

      const created = response?.data?.response ?? response?.data;
      if (!created?.id) {
        throw new Error("Create matchup response missing id");
      }

      // Use returned author_id so routing always matches backend access rules
      const createdAuthorId =
        created.author_id ?? created.authorId ?? created.AuthorID ?? authedUserId;

      navigate(`/users/${createdAuthorId}/matchup/${created.id}`);
    } catch (err) {
      console.error("Error creating matchup:", err);
      setError("We could not create that matchup. Please review your entries and try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="create-matchup-page">
      <NavigationBar />
      <main className="create-matchup-content">
        <section className="create-matchup-hero">
          <p className="create-overline">New Matchup</p>
          <h1>Bring a fresh head-to-head to the community.</h1>
          <p className="create-subtitle">
            Set the stage with a captivating title, tell everyone what the clash is about, and add
            contenders to get the debate started.
          </p>
          <div className="create-hero-actions">
            <Button onClick={() => navigate("/home")} className="create-secondary-button">
              Back to dashboard
            </Button>
            <Button onClick={goBack} className="create-tertiary-button">
              Go back
            </Button>
          </div>
        </section>

        <section className="create-layout">
          <form onSubmit={handleSubmit} className="create-form">
            <div className="create-form-group">
              <label htmlFor="matchup-title">
                Matchup title <span aria-hidden="true">*</span>
              </label>
              <input
                id="matchup-title"
                type="text"
                value={title}
                onChange={handleTitleChange}
                placeholder="e.g. Lakers vs Warriors: Who takes the series?"
                className="create-input"
                required
              />
            </div>

            <div className="create-form-group">
              <label htmlFor="matchup-content">
                Description <span aria-hidden="true">*</span>
              </label>
              <textarea
                id="matchup-content"
                value={content}
                onChange={handleContentChange}
                placeholder="Give everyone the context and criteria for your matchup."
                className="create-textarea"
                rows={6}
                required
              />
            </div>

            <div className="create-form-group">
              <label htmlFor="matchup-end-mode">
                How should this matchup end? <span aria-hidden="true">*</span>
              </label>
              <select
                id="matchup-end-mode"
                value={endMode}
                onChange={(e) => setEndMode(e.target.value)}
                className="create-input"
                required
              >
                <option value="manual">End manually (owner completes)</option>
                <option value="timer">End by timer</option>
              </select>
            </div>

            {endMode === "timer" && (
              <div className="create-form-group">
                <label>Timer duration</label>
                <div className="create-duration-row">
                  <div className="create-duration-field">
                    <input
                      type="number"
                      min={0}
                      max={1440}
                      value={durationMinutes}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setDurationMinutes(Number.isFinite(next) ? next : 0);
                      }}
                      className="create-input"
                      required
                    />
                    <span className="create-duration-label">Minutes</span>
                  </div>
                  <div className="create-duration-field">
                    <input
                      type="number"
                      min={0}
                      max={59}
                      value={durationSeconds}
                      onChange={(e) => {
                        const next = Number(e.target.value);
                        setDurationSeconds(Number.isFinite(next) ? next : 0);
                      }}
                      className="create-input"
                      required
                    />
                    <span className="create-duration-label">Seconds</span>
                  </div>
                </div>
              </div>
            )}

            <div className="create-form-group">
              <div className="create-form-group-header">
                <label>Contenders</label>
                <span className="create-form-hint">
                  Add at least two contenders to keep things interesting.
                </span>
              </div>

              <div className="create-items">
                {items.map((itm, index) => (
                  <div key={index} className="create-item-row">
                    <input
                      type="text"
                      value={itm.item}
                      onChange={(e) => handleItemChange(index, e.target.value)}
                      placeholder={`Contender ${index + 1}`}
                      className="create-input"
                      required
                    />
                    {items.length > 1 && (
                      <Button
                        type="button"
                        onClick={() => removeItem(index)}
                        className="create-remove-button"
                      >
                        Remove
                      </Button>
                    )}
                  </div>
                ))}
              </div>

              <Button
                type="button"
                onClick={addItem}
                className="create-add-button"
                disabled={items.length >= maxItems}
              >
                + Add another contender
              </Button>

              {items.length >= maxItems && (
                <p className="create-inline-hint" role="alert">
                  Standalone matchups can have up to {maxItems} contenders.
                </p>
              )}

              {!hasRequiredContenders && (
                <p className="create-inline-hint" role="alert">
                  You need at least two contenders to create a matchup.
                </p>
              )}

              {hasRequiredContenders && !allInputsFilled && (
                <p className="create-inline-hint" role="alert">
                  Please fill in every contender field (or remove empty ones).
                </p>
              )}

              {!Number.isFinite(authedUserId) || authedUserId <= 0 ? (
                <p className="create-inline-hint" role="alert">
                  You appear to be logged out. Please sign in again to create a matchup.
                </p>
              ) : null}
            </div>

            {error && <p className="create-error">{error}</p>}

            <div className="create-form-actions">
              <Button type="submit" className="create-primary-button" disabled={isCreateDisabled}>
                {isSubmitting ? "Creating..." : "Create matchup"}
              </Button>
              <Button type="button" onClick={goBack} className="create-tertiary-button">
                Cancel
              </Button>
            </div>
          </form>

          <aside className="create-preview-card">
            <div className="create-preview-header">
              <span className="create-preview-label">Live Preview</span>
              <span className="create-preview-count">{filledItems.length} contenders</span>
            </div>
            <div className="create-preview-body">
              <h2>{title.trim() || "Your matchup title will appear here"}</h2>
              <p>
                {content.trim() ||
                  "Use the description field to explain what makes this matchup exciting."}
              </p>
              <ul className="create-preview-list">
                {(filledItems.length > 0 ? filledItems : sanitizedItems).map(({ item }, index) => (
                  <li key={index}>
                    <span className="create-preview-bullet" aria-hidden="true" />
                    <span>{item.length > 0 ? item : `Contender ${index + 1}`}</span>
                  </li>
                ))}
              </ul>
            </div>
          </aside>
        </section>
      </main>
    </div>
  );
};

export default CreateMatchup;
