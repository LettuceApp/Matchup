import React, { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { FiChevronDown } from "react-icons/fi";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import { createMatchup } from "../services/api";

const CreateMatchup = () => {
  const { userId } = useParams();
  const navigate = useNavigate();

  // Prefer the authenticated user id (localStorage), fallback to route param
  const authedUserId = localStorage.getItem("userId") || userId || "";

  const minItems = 2;
  const maxItems = 4;

  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [items, setItems] = useState([{ item: "" }, { item: "" }]);
  const [endMode, setEndMode] = useState("manual");
  const [durationMinutes, setDurationMinutes] = useState(60);
  const [durationSeconds, setDurationSeconds] = useState(0);
  const [mutualsOnly, setMutualsOnly] = useState(false);
  const [tags, setTags] = useState("");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [showPreview, setShowPreview] = useState(true);
  const [touchedItems, setTouchedItems] = useState([false, false]);
  const [attemptedSubmit, setAttemptedSubmit] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState(null);
  const [successMessage, setSuccessMessage] = useState(null);

  const focusTransition = { type: "spring", stiffness: 260, damping: 20 };

  const handleTitleChange = (e) => setTitle(e.target.value);
  const handleContentChange = (e) => setContent(e.target.value);

  const handleItemChange = (index, value) => {
    setItems((prev) => prev.map((it, i) => (i === index ? { ...it, item: value } : it)));
  };

  const addItem = () => {
    if (items.length >= maxItems) return;
    setItems((prev) => [...prev, { item: "" }]);
    setTouchedItems((prev) => [...prev, false]);
  };

  const removeItem = (index) => {
    if (items.length <= minItems) return;
    setItems((prev) => prev.filter((_, i) => i !== index));
    setTouchedItems((prev) => prev.filter((_, i) => i !== index));
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

  const hasRequiredContenders = filledItems.length >= minItems;

  // All contender inputs must be non-empty (no blank rows)
  const allInputsFilled = filledItems.length === items.length;

  const isCreateDisabled =
    isSubmitting ||
    title.trim().length === 0 ||
    content.trim().length === 0 ||
    !allInputsFilled ||
    !hasRequiredContenders ||
    !authedUserId;

  const disabledReason = useMemo(() => {
    if (!authedUserId) {
      return "Please sign in again to create a matchup.";
    }
    if (title.trim().length === 0) {
      return "Add a title to continue.";
    }
    if (content.trim().length === 0) {
      return "Add a description to continue.";
    }
    if (filledItems.length < minItems) {
      return "You need at least two contenders to create a matchup.";
    }
    if (!allInputsFilled) {
      return "Each contender needs a name before you can create this matchup.";
    }
    if (isSubmitting) {
      return "Creating matchup...";
    }
    return "";
  }, [allInputsFilled, authedUserId, content, filledItems.length, isSubmitting, minItems, title]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setAttemptedSubmit(true);
    if (isCreateDisabled) return;

    setError(null);
    setSuccessMessage(null);

    const matchupData = {
      title: title.trim(),
      content: content.trim(),
      items: sanitizedItems,
      status: "draft",
      end_mode: endMode,
    };
    if (mutualsOnly) {
      matchupData.visibility = "mutuals";
    }
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
      const authorSlug = created?.author?.username || createdAuthorId;

      setSuccessMessage("Matchup created! Redirecting you to the debate...");
      setTimeout(() => {
        navigate(`/users/${authorSlug}/matchup/${created.id}`);
      }, 450);
    } catch (err) {
      console.error("Error creating matchup:", err);
      setError("We could not create that matchup. Please review your entries and try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const previewItems = filledItems.length > 0 ? filledItems : sanitizedItems;
  const showItemError = (index) =>
    (attemptedSubmit || touchedItems[index]) && sanitizedItems[index].item.length === 0;

  return (
    <div
      className="min-h-screen text-slate-100"
      style={{
        background:
          "radial-gradient(120% 120% at 0% 0%, rgba(59, 130, 246, 0.35) 0%, rgba(15, 23, 42, 0) 45%), radial-gradient(120% 120% at 100% 0%, rgba(244, 114, 182, 0.35) 0%, rgba(15, 23, 42, 0) 45%), linear-gradient(135deg, #0f172a 0%, #111c3d 100%)",
      }}
    >
      <NavigationBar />
      <main className="mx-auto flex w-full max-w-6xl flex-col gap-10 px-6 pb-16 pt-24">
        <motion.section
          className="relative overflow-hidden rounded-3xl border border-slate-700/60 bg-slate-900/60 px-6 py-8 shadow-[0_32px_70px_rgba(15,23,42,0.55)] sm:px-10"
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45 }}
        >
          <p className="text-xs font-semibold uppercase tracking-[0.3em] text-slate-300/80">
            New matchup
          </p>
          <h1 className="mt-3 text-3xl font-semibold leading-tight text-slate-50 sm:text-4xl">
            Build a debate that feels good to join.
          </h1>
          <p className="mt-3 max-w-2xl text-base leading-relaxed text-slate-200/80">
            Start with the essentials, add 2–4 contenders, then open advanced settings only if you
            need more control.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button
              onClick={() => navigate("/home")}
              className="rounded-full border border-slate-600/60 bg-transparent px-5 py-2 text-sm font-semibold text-slate-100 hover:border-slate-400/70"
            >
              Back to dashboard
            </Button>
            <Button
              onClick={goBack}
              className="rounded-full border border-slate-700/50 bg-slate-950/40 px-5 py-2 text-sm font-semibold text-slate-200 hover:border-slate-400/70"
            >
              Go back
            </Button>
          </div>
        </motion.section>

        <section className="grid gap-8 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
          <motion.form
            onSubmit={handleSubmit}
            className="flex flex-col gap-6 rounded-3xl border border-slate-700/60 bg-slate-900/70 p-6 shadow-[0_28px_60px_rgba(15,23,42,0.55)] sm:p-8"
            initial={{ opacity: 0, y: 18 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.4, delay: 0.05 }}
          >
            <div className="flex flex-col gap-3">
              <label className="text-sm font-semibold text-slate-100" htmlFor="matchup-title">
                Matchup title <span className="text-amber-300">*</span>
              </label>
              <motion.input
                id="matchup-title"
                type="text"
                value={title}
                onChange={handleTitleChange}
                placeholder="e.g. Lakers vs Warriors: Who takes the series?"
                className="w-full rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-base text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                whileFocus={{ scale: 1.01 }}
                transition={focusTransition}
                required
              />
            </div>

            <div className="flex flex-col gap-3">
              <label className="text-sm font-semibold text-slate-100" htmlFor="matchup-content">
                Description <span className="text-amber-300">*</span>
              </label>
              <motion.textarea
                id="matchup-content"
                value={content}
                onChange={handleContentChange}
                placeholder="Give everyone the context and criteria for your matchup."
                className="min-h-[160px] w-full resize-y rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-base text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                rows={6}
                whileFocus={{ scale: 1.01 }}
                transition={focusTransition}
                required
              />
            </div>

            <div className="flex flex-col gap-3">
              <div className="flex flex-col gap-1">
                <label className="text-sm font-semibold text-slate-100">Contenders</label>
                <span className="text-sm text-slate-300/80">
                  Add 2–4 contenders for a good debate.
                </span>
              </div>

              <div className="flex flex-col gap-3">
                {items.map((itm, index) => (
                  <div key={index} className="flex flex-col gap-2">
                    <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                      <motion.input
                        type="text"
                        value={itm.item}
                        onChange={(e) => handleItemChange(index, e.target.value)}
                        onBlur={() =>
                          setTouchedItems((prev) =>
                            prev.map((t, i) => (i === index ? true : t))
                          )
                        }
                        placeholder={`Contender ${index + 1}`}
                        className="flex-1 rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-base text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                        whileFocus={{ scale: 1.01 }}
                        transition={focusTransition}
                        required
                      />
                      {items.length > minItems && (
                        <Button
                          type="button"
                          onClick={() => removeItem(index)}
                          className="rounded-full border border-rose-400/70 bg-rose-500/10 px-4 py-2 text-sm font-semibold text-rose-100 hover:border-rose-300/80"
                        >
                          Remove
                        </Button>
                      )}
                    </div>
                    {showItemError(index) && (
                      <p className="text-sm font-medium text-amber-300">
                        Contender name required.
                      </p>
                    )}
                  </div>
                ))}
              </div>

              {items.length < maxItems && (
                <Button
                  type="button"
                  onClick={addItem}
                  className="w-fit rounded-full border border-slate-500/70 bg-slate-900/40 px-4 py-2 text-sm font-semibold text-slate-100 hover:border-blue-300/70"
                >
                  + Add another contender
                </Button>
              )}

              {items.length >= maxItems && (
                <p className="text-sm font-medium text-slate-300/80">
                  Maximum of {maxItems} contenders reached.
                </p>
              )}
            </div>

            <div className="rounded-2xl border border-slate-700/60 bg-slate-950/40">
              <button
                type="button"
                onClick={() => setAdvancedOpen((prev) => !prev)}
                className="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold text-slate-100"
              >
                <span>Advanced settings</span>
                <motion.span
                  animate={{ rotate: advancedOpen ? 180 : 0 }}
                  transition={{ duration: 0.2 }}
                  className="text-slate-300"
                >
                  <FiChevronDown />
                </motion.span>
              </button>

              <AnimatePresence initial={false}>
                {advancedOpen && (
                  <motion.div
                    key="advanced-panel"
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    transition={{ duration: 0.25 }}
                    className="overflow-hidden border-t border-slate-700/60"
                  >
                    <div className="flex flex-col gap-4 px-4 py-4">
                      <div className="flex flex-col gap-2">
                        <label className="text-sm font-semibold text-slate-100">
                          Matchup end mode
                        </label>
                        <select
                          value={endMode}
                          onChange={(e) => setEndMode(e.target.value)}
                          className="w-full rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                        >
                          <option value="manual">End manually</option>
                          <option value="timer">End by timer</option>
                        </select>
                      </div>

                      <AnimatePresence initial={false}>
                        {endMode === "timer" && (
                          <motion.div
                            key="timer-fields"
                            initial={{ opacity: 0, y: -6 }}
                            animate={{ opacity: 1, y: 0 }}
                            exit={{ opacity: 0, y: -6 }}
                            transition={{ duration: 0.2 }}
                            className="grid gap-3 sm:grid-cols-2"
                          >
                            <div className="flex flex-col gap-2">
                              <label className="text-xs font-semibold text-slate-300">
                                Minutes
                              </label>
                              <motion.input
                                type="number"
                                min={0}
                                max={1440}
                                value={durationMinutes}
                                onChange={(e) => {
                                  const next = Number(e.target.value);
                                  setDurationMinutes(Number.isFinite(next) ? next : 0);
                                }}
                                className="rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                                whileFocus={{ scale: 1.01 }}
                                transition={focusTransition}
                                required
                              />
                            </div>
                            <div className="flex flex-col gap-2">
                              <label className="text-xs font-semibold text-slate-300">
                                Seconds
                              </label>
                              <motion.input
                                type="number"
                                min={0}
                                max={59}
                                value={durationSeconds}
                                onChange={(e) => {
                                  const next = Number(e.target.value);
                                  setDurationSeconds(Number.isFinite(next) ? next : 0);
                                }}
                                className="rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-sm text-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                                whileFocus={{ scale: 1.01 }}
                                transition={focusTransition}
                                required
                              />
                            </div>
                          </motion.div>
                        )}
                      </AnimatePresence>

                      <label className="flex items-center gap-3 rounded-2xl border border-slate-600/60 bg-slate-950/60 px-4 py-3 text-sm font-semibold text-slate-100">
                        <input
                          type="checkbox"
                          checked={mutualsOnly}
                          onChange={(e) => setMutualsOnly(e.target.checked)}
                          className="h-4 w-4 accent-blue-400"
                        />
                        Mutuals only (limit access to mutual followers)
                      </label>

                      <div className="flex flex-col gap-2">
                        <label className="text-sm font-semibold text-slate-100">
                          Tags (optional)
                        </label>
                        <motion.input
                          type="text"
                          value={tags}
                          onChange={(e) => setTags(e.target.value)}
                          placeholder="Pop culture, sports, music"
                          className="w-full rounded-2xl border border-slate-600/70 bg-slate-950/60 px-4 py-3 text-sm text-slate-100 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-blue-400/60"
                          whileFocus={{ scale: 1.01 }}
                          transition={focusTransition}
                        />
                      </div>
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
            </div>

            {error && (
              <p className="rounded-2xl border border-rose-400/70 bg-rose-500/10 px-4 py-3 text-sm font-semibold text-rose-100">
                {error}
              </p>
            )}
            {successMessage && (
              <p className="rounded-2xl border border-emerald-400/60 bg-emerald-500/10 px-4 py-3 text-sm font-semibold text-emerald-100">
                {successMessage}
              </p>
            )}

            <div className="flex flex-col gap-2">
              <div className="flex flex-wrap gap-3">
                <Button
                  type="submit"
                  className="rounded-full bg-gradient-to-r from-amber-400 to-orange-400 px-6 py-3 text-sm font-semibold text-slate-900 shadow-lg shadow-orange-500/30 disabled:cursor-not-allowed disabled:opacity-60"
                  disabled={isCreateDisabled}
                >
                  {isSubmitting ? "Creating..." : "Create matchup"}
                </Button>
                <Button
                  type="button"
                  onClick={goBack}
                  className="rounded-full border border-slate-600/60 bg-slate-950/30 px-6 py-3 text-sm font-semibold text-slate-100 hover:border-slate-400/70"
                >
                  Cancel
                </Button>
                <Button
                  type="button"
                  onClick={() => setShowPreview((prev) => !prev)}
                  className="rounded-full border border-slate-600/60 bg-slate-950/30 px-6 py-3 text-sm font-semibold text-slate-100 hover:border-slate-400/70 lg:hidden"
                >
                  {showPreview ? "Hide preview" : "Show preview"}
                </Button>
              </div>
              {isCreateDisabled && disabledReason && (
                <p className="text-sm font-semibold text-amber-300">{disabledReason}</p>
              )}
            </div>
          </motion.form>

          <motion.aside
            className={`${showPreview ? "block" : "hidden"} lg:block`}
            initial={{ opacity: 0, y: 18 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.4, delay: 0.1 }}
          >
            <div className="sticky top-24 flex flex-col gap-4 rounded-3xl border border-slate-700/60 bg-slate-900/70 p-6 shadow-[0_28px_60px_rgba(15,23,42,0.45)]">
              <div className="flex items-center justify-between text-sm font-semibold text-slate-200">
                <span>Live preview</span>
                <span className="rounded-full border border-slate-600/60 px-3 py-1 text-xs text-slate-300">
                  {filledItems.length}/{maxItems} contenders
                </span>
              </div>
              <AnimatePresence mode="wait">
                <motion.div
                  key={`${title}-${content}-${previewItems.length}`}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  transition={{ duration: 0.2 }}
                  className="flex flex-col gap-3"
                >
                  <div>
                    <h2 className="text-xl font-semibold text-slate-50">
                      {title.trim() || "Your matchup title will appear here"}
                    </h2>
                    <p className="mt-2 text-sm text-slate-300/80">
                      {content.trim() ||
                        "Use the description to help voters understand the stakes."}
                    </p>
                  </div>
                  {mutualsOnly && (
                    <span className="w-fit rounded-full border border-blue-400/60 bg-blue-500/10 px-3 py-1 text-xs font-semibold text-blue-100">
                      Mutuals only
                    </span>
                  )}
                  <div className="rounded-2xl border border-slate-700/60 bg-slate-950/40 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-slate-400">
                      Contenders
                    </p>
                    <ul className="mt-3 flex flex-col gap-2 text-sm text-slate-100">
                      {previewItems.map(({ item }, index) => (
                        <li key={index} className="flex items-center gap-2">
                          <span className="h-2 w-2 rounded-full bg-amber-400" aria-hidden="true" />
                          <span>{item.length > 0 ? item : `Contender ${index + 1}`}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                </motion.div>
              </AnimatePresence>
            </div>
          </motion.aside>
        </section>
      </main>
    </div>
  );
};

export default CreateMatchup;
