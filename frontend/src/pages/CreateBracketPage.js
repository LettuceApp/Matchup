import React, { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { AnimatePresence, motion } from "framer-motion";
import { Bracket } from "react-tournament-bracket";
import {
  FiAlertCircle,
  FiCheckCircle,
  FiChevronDown,
  FiChevronRight,
  FiShuffle,
  FiUpload,
} from "react-icons/fi";
import NavigationBar from "../components/NavigationBar";
import Button from "../components/Button";
import { createBracket } from "../services/api";

const sizeOptions = [4, 8, 16, 32, 64];
const advanceModes = [
  { value: "manual", label: "Manual (owner advances)" },
  { value: "timer", label: "Timer (round clock)" },
];

const stepLabels = [
  { id: 1, label: "Basics" },
  { id: 2, label: "Seeds" },
  { id: 3, label: "Preview" },
  { id: 4, label: "Review" },
];

const buildSeedOrder = (size) => {
  if (size <= 1) return [];
  let order = [1, 2];
  while (order.length < size) {
    const nextSize = order.length * 2;
    const next = [];
    order.forEach((seed) => {
      next.push(seed);
      next.push(nextSize + 1 - seed);
    });
    order = next;
  }
  return order;
};

const buildBracketGame = (seedOrder, entries) => {
  const scheduled = Date.now();
  const roundCount = Math.log2(seedOrder.length);
  const makeSeedLabel = (seedIndex) => {
    const name = entries[seedIndex - 1];
    return name ? `#${seedIndex} ${name}` : `Seed ${seedIndex}`;
  };

  let roundGames = [];
  for (let i = 0; i < seedOrder.length; i += 2) {
    const seedA = seedOrder[i];
    const seedB = seedOrder[i + 1];
    roundGames.push({
      id: `r1m${i / 2 + 1}`,
      name: "Round 1",
      scheduled,
      sides: {
        home: {
          team: {
            id: `seed-${seedA}`,
            name: makeSeedLabel(seedA),
          },
        },
        visitor: {
          team: {
            id: `seed-${seedB}`,
            name: makeSeedLabel(seedB),
          },
        },
      },
    });
  }

  for (let round = 2; round <= roundCount; round += 1) {
    const next = [];
    for (let i = 0; i < roundGames.length; i += 2) {
      const homeSource = roundGames[i];
      const visitorSource = roundGames[i + 1];
      next.push({
        id: `r${round}m${i / 2 + 1}`,
        name: `Round ${round}`,
        scheduled,
        sides: {
          home: {
            seed: {
              displayName: `Winner R${round - 1}M${i + 1}`,
              rank: 1,
              sourceGame: homeSource,
              sourcePool: {},
            },
          },
          visitor: {
            seed: {
              displayName: `Winner R${round - 1}M${i + 2}`,
              rank: 1,
              sourceGame: visitorSource,
              sourcePool: {},
            },
          },
        },
      });
    }
    roundGames = next;
  }

  return roundGames[0];
};

const parseEntryList = (text) =>
  text
    .split(/\r?\n|,/)
    .map((entry) => entry.trim())
    .filter(Boolean);

const CreateBracketPage = () => {
  const navigate = useNavigate();
  const authedUserId = localStorage.getItem("userId") || "";

  const [step, setStep] = useState(1);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [size, setSize] = useState(8);
  const [advanceMode, setAdvanceMode] = useState("manual");
  const [roundMinutes, setRoundMinutes] = useState("");
  const [roundSeconds, setRoundSeconds] = useState("");
  const [mutualsOnly, setMutualsOnly] = useState(false);
  const [tags, setTags] = useState("");
  const [notes, setNotes] = useState("");
  const [entries, setEntries] = useState(Array(8).fill(""));
  const [touchedSeeds, setTouchedSeeds] = useState(Array(8).fill(false));
  const [bulkText, setBulkText] = useState("");
  const [bulkError, setBulkError] = useState("");
  const [openGroups, setOpenGroups] = useState({ 0: true });
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [showPreview, setShowPreview] = useState(true);
  const [attemptedSubmit, setAttemptedSubmit] = useState(false);
  const [error, setError] = useState(null);
  const [successMessage, setSuccessMessage] = useState(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const isLargeSize = size >= 16;
  const groupSize = isLargeSize ? 8 : size;
  const groupCount = Math.ceil(size / groupSize);

  useEffect(() => {
    setEntries((prev) => {
      const next = Array(size).fill("");
      prev.slice(0, size).forEach((value, index) => {
        next[index] = value;
      });
      return next;
    });
    setTouchedSeeds((prev) => {
      const next = Array(size).fill(false);
      prev.slice(0, size).forEach((value, index) => {
        next[index] = value;
      });
      return next;
    });
    setOpenGroups({ 0: true });
    setBulkText("");
    setBulkError("");
  }, [size]);

  const trimmedEntries = useMemo(
    () => entries.map((entry) => (entry ?? "").trim()),
    [entries]
  );

  const filledCount = useMemo(
    () => trimmedEntries.filter((entry) => entry.length > 0).length,
    [trimmedEntries]
  );

  const missingSeeds = useMemo(
    () =>
      trimmedEntries
        .map((entry, index) => (entry.length === 0 ? index + 1 : null))
        .filter(Boolean),
    [trimmedEntries]
  );

  const duplicateEntries = useMemo(() => {
    const counts = new Map();
    trimmedEntries.forEach((entry) => {
      if (!entry) return;
      const key = entry.toLowerCase();
      counts.set(key, (counts.get(key) || 0) + 1);
    });
    return Array.from(counts.entries())
      .filter(([, count]) => count > 1)
      .map(([value]) => value);
  }, [trimmedEntries]);

  const totalSeconds = useMemo(() => {
    const minutes = Number.parseInt(roundMinutes, 10);
    const seconds = Number.parseInt(roundSeconds, 10);
    const safeMinutes = Number.isFinite(minutes) ? minutes : 0;
    const safeSeconds = Number.isFinite(seconds) ? seconds : 0;
    return safeMinutes * 60 + safeSeconds;
  }, [roundMinutes, roundSeconds]);

  const timerValid = advanceMode !== "timer" || (totalSeconds >= 60 && totalSeconds <= 86400);
  const basicsReady = title.trim().length > 0 && description.trim().length > 0;
  const hasMinimumSeeds = filledCount >= 2;
  const allSeedsFilled = filledCount === size;
  const hasDuplicates = duplicateEntries.length > 0;

  const canCreate =
    basicsReady &&
    allSeedsFilled &&
    !hasDuplicates &&
    timerValid &&
    !isSubmitting &&
    authedUserId;

  const previewSeedOrder = useMemo(() => buildSeedOrder(size), [size]);

  const previewGame = useMemo(
    () => buildBracketGame(previewSeedOrder, trimmedEntries),
    [previewSeedOrder, trimmedEntries]
  );

  const previewStyles = useMemo(
    () => ({
      backgroundColor: "rgba(15, 23, 42, 0.85)",
      hoverBackgroundColor: "rgba(30, 41, 59, 0.9)",
      scoreBackground: "rgba(15, 23, 42, 0.8)",
      winningScoreBackground: "rgba(251, 191, 36, 0.9)",
      teamNameStyle: {
        fill: "#e2e8f0",
        fontSize: 12,
        fontWeight: 600,
      },
      teamScoreStyle: { fill: "#0f172a", fontSize: 10 },
      gameNameStyle: { fill: "#94a3b8", fontSize: 9 },
      gameTimeStyle: { fill: "#64748b", fontSize: 9 },
      teamSeparatorStyle: { stroke: "rgba(51,65,85,0.6)", strokeWidth: 1 },
    }),
    []
  );

  const handleEntryChange = (index, value) => {
    setEntries((prev) => prev.map((entry, idx) => (idx === index ? value : entry)));
    setTouchedSeeds((prev) => prev.map((touched, idx) => (idx === index ? true : touched)));
  };

  const handleBulkApply = () => {
    const parsed = parseEntryList(bulkText);
    if (parsed.length < 2) {
      setBulkError("Paste at least two contenders to continue.");
      return;
    }
    if (parsed.length > size) {
      setBulkError(`You provided ${parsed.length} contenders. This bracket needs ${size}.`);
      return;
    }
    const next = Array(size).fill("");
    parsed.forEach((entry, index) => {
      next[index] = entry;
    });
    setEntries(next);
    setTouchedSeeds(next.map((entry) => Boolean(entry)));
    setBulkError("");
  };

  const handleRandomize = () => {
    const activeEntries = trimmedEntries.filter(Boolean);
    if (activeEntries.length < 2) {
      setBulkError("Add at least two contenders before randomizing.");
      return;
    }
    const shuffled = [...activeEntries];
    for (let i = shuffled.length - 1; i > 0; i -= 1) {
      const j = Math.floor(Math.random() * (i + 1));
      [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
    }
    const next = Array(size).fill("");
    shuffled.forEach((entry, index) => {
      next[index] = entry;
    });
    setEntries(next);
    setTouchedSeeds(next.map((entry) => Boolean(entry)));
    setBulkError("");
  };

  const handleFileUpload = (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (e) => {
      const text = e.target?.result;
      if (typeof text === "string") {
        setBulkText(text);
        const parsed = parseEntryList(text);
        if (parsed.length === 0) {
          setBulkError("That CSV did not include any names.");
          return;
        }
        if (parsed.length > size) {
          setBulkError(`That CSV includes ${parsed.length} contenders. This bracket needs ${size}.`);
          return;
        }
        const next = Array(size).fill("");
        parsed.forEach((entry, index) => {
          next[index] = entry;
        });
        setEntries(next);
        setTouchedSeeds(next.map((entry) => Boolean(entry)));
        setBulkError("");
      }
    };
    reader.readAsText(file);
  };

  const toggleGroup = (groupIndex) => {
    setOpenGroups((prev) => ({ ...prev, [groupIndex]: !prev[groupIndex] }));
  };

  const nextStep = () => {
    if (step === 1 && !basicsReady) {
      setAttemptedSubmit(true);
      return;
    }
    if (step === 2 && !hasMinimumSeeds) {
      setAttemptedSubmit(true);
      return;
    }
    setAttemptedSubmit(false);
    setStep((prev) => Math.min(4, prev + 1));
  };

  const prevStep = () => {
    setStep((prev) => Math.max(1, prev - 1));
  };

  const handleSubmit = async (event) => {
    event.preventDefault();
    setAttemptedSubmit(true);
    if (!canCreate) return;

    setError(null);
    setSuccessMessage(null);
    setIsSubmitting(true);

    const payload = {
      title: title.trim(),
      description: description.trim(),
      size,
      advance_mode: advanceMode,
      entries: trimmedEntries,
    };
    if (mutualsOnly) {
      payload.visibility = "mutuals";
    }
    if (advanceMode === "timer") {
      payload.round_duration_seconds = totalSeconds;
    }

    try {
      const response = await createBracket(authedUserId, payload);
      const created = response?.data?.response ?? response?.data;
      if (!created?.id) {
        throw new Error("Create bracket response missing id");
      }
      setSuccessMessage("Bracket created! Taking you to the preview...");
      setTimeout(() => {
        navigate(`/brackets/${created.id}`);
      }, 500);
    } catch (err) {
      console.error("Error creating bracket:", err);
      setError("We could not create that bracket. Please review your entries and try again.");
    } finally {
      setIsSubmitting(false);
    }
  };

  const seedInputError = (index) =>
    (attemptedSubmit || touchedSeeds[index]) && trimmedEntries[index].length === 0;

  const previewCard = (
    <motion.aside
      className="relative min-w-0 overflow-hidden rounded-3xl border border-slate-700/60 bg-slate-900/65 p-6 shadow-[0_24px_60px_rgba(15,23,42,0.45)]"
      layout
      initial={{ opacity: 0, y: 16 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4 }}
    >
      <div className="flex flex-col gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.3em] text-slate-300/70">
            Live preview
          </p>
          <h2 className="mt-2 text-2xl font-semibold text-slate-50">
            {title.trim() || "Bracket title preview"}
          </h2>
          <p className="mt-2 text-sm text-slate-200/70">
            {description.trim() || "Describe the debate to help voters jump in."}
          </p>
          <div className="mt-4 flex flex-wrap gap-3 text-xs font-semibold text-slate-200/70">
            <span className="rounded-full border border-slate-600/50 px-3 py-1">
              {filledCount}/{size} contenders
            </span>
            <span className="rounded-full border border-slate-600/50 px-3 py-1">
              {advanceMode === "manual" ? "Manual advance" : "Timer advance"}
            </span>
            {mutualsOnly && (
              <span className="rounded-full border border-amber-400/50 px-3 py-1 text-amber-200">
                Mutuals only
              </span>
            )}
          </div>
        </div>
        <div className="rounded-2xl border border-slate-700/60 bg-slate-950/50 p-4">
          <div className="mb-3 flex items-center justify-between text-xs uppercase tracking-[0.2em] text-slate-300/60">
            <span>Bracket preview</span>
            <span>{size}-team</span>
          </div>
          <div className="h-[360px] min-w-0 overflow-auto pr-2">
            <Bracket
              game={previewGame}
              styles={previewStyles}
              gameDimensions={{ height: 84, width: 220 }}
              roundSeparatorWidth={28}
              svgPadding={18}
            />
          </div>
        </div>
      </div>
    </motion.aside>
  );

  return (
    <div
      className="min-h-screen overflow-x-hidden text-slate-100"
      style={{
        background:
          "radial-gradient(120% 120% at 0% 0%, rgba(59, 130, 246, 0.35) 0%, rgba(15, 23, 42, 0) 45%), radial-gradient(120% 120% at 100% 0%, rgba(244, 114, 182, 0.35) 0%, rgba(15, 23, 42, 0) 45%), linear-gradient(135deg, #0f172a 0%, #111c3d 100%)",
      }}
    >
      <NavigationBar />
      <main className="mx-auto flex w-full max-w-7xl flex-col gap-8 px-6 pb-16 pt-24 xl:px-10">
        <motion.section
          className="rounded-3xl border border-slate-700/60 bg-slate-900/60 px-6 py-8 shadow-[0_32px_70px_rgba(15,23,42,0.55)] sm:px-10"
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 0.45 }}
        >
          <p className="text-xs font-semibold uppercase tracking-[0.3em] text-slate-300/80">
            Create a bracket
          </p>
          <h1 className="mt-3 text-3xl font-semibold leading-tight text-slate-50 sm:text-4xl">
            Build a tournament that feels easy to follow.
          </h1>
          <p className="mt-3 max-w-2xl text-base text-slate-200/80">
            Start with the basics, add your contenders, then preview the bracket live before you
            publish.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button
              onClick={() => navigate("/home")}
              className="rounded-full border border-slate-600/60 bg-transparent px-5 py-2 text-sm font-semibold text-slate-100 hover:border-slate-400/70"
            >
              Back to dashboard
            </Button>
            <Button
              onClick={() => navigate(-1)}
              className="rounded-full border border-slate-700/50 bg-slate-950/40 px-5 py-2 text-sm font-semibold text-slate-200 hover:border-slate-400/70"
            >
              Go back
            </Button>
          </div>
        </motion.section>

        <section className="flex flex-wrap gap-3">
          {stepLabels.map((item) => {
            const isActive = step === item.id;
            const isComplete = step > item.id;
            return (
              <div
                key={item.id}
                className={`flex items-center gap-3 rounded-full border px-4 py-2 text-sm font-semibold ${
                  isActive
                    ? "border-sky-400/70 bg-sky-500/10 text-sky-100"
                    : isComplete
                      ? "border-emerald-400/60 bg-emerald-500/10 text-emerald-100"
                      : "border-slate-700/70 bg-slate-900/60 text-slate-200/70"
                }`}
              >
                <span
                  className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-bold ${
                    isComplete
                      ? "bg-emerald-500/20 text-emerald-200"
                      : "bg-slate-800/60 text-slate-200"
                  }`}
                >
                  {item.id}
                </span>
                {item.label}
              </div>
            );
          })}
        </section>

        <div className="grid min-w-0 gap-8 lg:grid-cols-[1.15fr_0.85fr]">
          <motion.form
            className="min-w-0 rounded-3xl border border-slate-700/60 bg-slate-900/70 p-6 shadow-[0_24px_60px_rgba(15,23,42,0.45)] sm:p-8"
            onSubmit={handleSubmit}
            layout
          >
            <AnimatePresence mode="wait">
              {step === 1 && (
                <motion.div
                  key="step-1"
                  initial={{ opacity: 0, x: -12 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 12 }}
                  transition={{ duration: 0.35 }}
                  className="flex flex-col gap-6"
                >
                  <div>
                    <h2 className="text-2xl font-semibold text-slate-50">Step 1 - Basics</h2>
                    <p className="mt-2 text-sm text-slate-200/70">
                      Add the essentials first so your bracket feels intentional.
                    </p>
                  </div>
                  <div className="flex flex-col gap-4">
                    <label className="text-sm font-semibold text-slate-200">Title</label>
                    <input
                      type="text"
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                      className="rounded-2xl border border-slate-700/60 bg-slate-950/50 px-4 py-3 text-base text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                      placeholder="Best late-night snack"
                    />
                    {attemptedSubmit && title.trim().length === 0 && (
                      <p className="text-xs text-rose-300">Title is required.</p>
                    )}
                  </div>
                  <div className="flex flex-col gap-4">
                    <label className="text-sm font-semibold text-slate-200">Description</label>
                    <textarea
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                      rows={3}
                      className="rounded-2xl border border-slate-700/60 bg-slate-950/50 px-4 py-3 text-base text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                      placeholder="Share a quick note about what voters should consider."
                    />
                    {attemptedSubmit && description.trim().length === 0 && (
                      <p className="text-xs text-rose-300">Description is required.</p>
                    )}
                  </div>
                  <div className="grid gap-4 sm:grid-cols-2">
                    <div className="flex flex-col gap-3">
                      <label className="text-sm font-semibold text-slate-200">Bracket size</label>
                      <select
                        value={size}
                        onChange={(e) => setSize(Number(e.target.value))}
                        className="rounded-2xl border border-slate-700/60 bg-slate-950/50 px-4 py-3 text-base text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                      >
                        {sizeOptions.map((option) => (
                          <option key={option} value={option}>
                            {option} contenders
                          </option>
                        ))}
                      </select>
                    </div>
                    <div className="flex flex-col gap-3">
                      <label className="text-sm font-semibold text-slate-200">
                        Round advancement
                      </label>
                      <select
                        value={advanceMode}
                        onChange={(e) => setAdvanceMode(e.target.value)}
                        className="rounded-2xl border border-slate-700/60 bg-slate-950/50 px-4 py-3 text-base text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                      >
                        {advanceModes.map((option) => (
                          <option key={option.value} value={option.value}>
                            {option.label}
                          </option>
                        ))}
                      </select>
                    </div>
                  </div>
                  <p className="text-sm text-slate-200/70">
                    Manual is the default. Switch to timer only if you want rounds to auto-advance.
                  </p>
                  {advanceMode === "timer" && (
                    <p className="text-xs text-amber-200">
                      Set the round duration inside Advanced settings before publishing.
                    </p>
                  )}
                </motion.div>
              )}

              {step === 2 && (
                <motion.div
                  key="step-2"
                  initial={{ opacity: 0, x: -12 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 12 }}
                  transition={{ duration: 0.35 }}
                  className="flex flex-col gap-6"
                >
                  <div>
                    <h2 className="text-2xl font-semibold text-slate-50">Step 2 - Seeds</h2>
                    <p className="mt-2 text-sm text-slate-200/70">
                      Add {size} contenders for a strong bracket. You can paste a list or fill them
                      manually.
                    </p>
                  </div>

                  {isLargeSize && (
                    <div className="rounded-2xl border border-slate-700/60 bg-slate-950/40 p-4">
                      <div className="flex flex-wrap items-center justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold text-slate-100">
                            Bulk entry tools
                          </p>
                          <p className="text-xs text-slate-200/70">
                            Paste contenders (one per line) or upload a CSV.
                          </p>
                        </div>
                        <div className="flex flex-wrap gap-2">
                          <Button
                            type="button"
                            onClick={handleBulkApply}
                            className="rounded-full border border-slate-600/60 bg-slate-950/70 px-4 py-2 text-xs font-semibold text-slate-100 hover:border-sky-400/70"
                          >
                            Apply list
                          </Button>
                          <Button
                            type="button"
                            onClick={handleRandomize}
                            className="flex items-center gap-2 rounded-full border border-slate-600/60 bg-slate-950/70 px-4 py-2 text-xs font-semibold text-slate-100 hover:border-sky-400/70"
                          >
                            <FiShuffle />
                            Randomize seeds
                          </Button>
                        </div>
                      </div>
                      <textarea
                        value={bulkText}
                        onChange={(e) => setBulkText(e.target.value)}
                        rows={4}
                        className="mt-4 w-full rounded-2xl border border-slate-700/60 bg-slate-900/60 px-4 py-3 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                        placeholder="Paste team names here (one per line)"
                      />
                      <div className="mt-3 flex flex-wrap items-center justify-between gap-3 text-xs text-slate-200/70">
                        <label className="flex cursor-pointer items-center gap-2">
                          <FiUpload />
                          Upload CSV
                          <input
                            type="file"
                            accept=".csv,text/csv"
                            onChange={handleFileUpload}
                            className="hidden"
                          />
                        </label>
                        <span>{filledCount}/{size} seeded</span>
                      </div>
                      {bulkError && (
                        <p className="mt-3 flex items-center gap-2 text-xs text-rose-300">
                          <FiAlertCircle />
                          {bulkError}
                        </p>
                      )}
                    </div>
                  )}

                  {!isLargeSize && (
                    <div className="grid gap-4 sm:grid-cols-2">
                      {Array.from({ length: size }).map((_, index) => (
                        <div key={`seed-${index}`} className="flex flex-col gap-2">
                          <label className="text-xs font-semibold text-slate-200/70">
                            Seed {index + 1}
                          </label>
                          <input
                            type="text"
                            value={entries[index]}
                            onChange={(e) => handleEntryChange(index, e.target.value)}
                            className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                            placeholder={`Contender ${index + 1}`}
                          />
                          {seedInputError(index) && (
                            <p className="text-xs text-rose-300">Contender name required.</p>
                          )}
                        </div>
                      ))}
                    </div>
                  )}

                  {isLargeSize && (
                    <div className="flex max-h-[480px] flex-col gap-4 overflow-auto pr-2">
                      {Array.from({ length: groupCount }).map((_, groupIndex) => {
                        const start = groupIndex * groupSize;
                        const end = Math.min(size, start + groupSize);
                        const groupEntries = trimmedEntries.slice(start, end);
                        const filledInGroup = groupEntries.filter(Boolean).length;
                        const isOpen = openGroups[groupIndex];

                        return (
                          <div
                            key={`group-${groupIndex}`}
                            className="rounded-2xl border border-slate-700/50 bg-slate-950/40"
                          >
                            <button
                              type="button"
                              onClick={() => toggleGroup(groupIndex)}
                              className="flex w-full items-center justify-between px-4 py-3 text-left text-sm font-semibold text-slate-100"
                            >
                              <span>
                                Group {groupIndex + 1} - Seeds {start + 1} to {end}
                              </span>
                              <span className="flex items-center gap-2 text-xs text-slate-200/70">
                                {filledInGroup}/{end - start} filled
                                {isOpen ? <FiChevronDown /> : <FiChevronRight />}
                              </span>
                            </button>
                            <AnimatePresence>
                              {isOpen && (
                                <motion.div
                                  initial={{ height: 0, opacity: 0 }}
                                  animate={{ height: "auto", opacity: 1 }}
                                  exit={{ height: 0, opacity: 0 }}
                                  transition={{ duration: 0.3 }}
                                  className="border-t border-slate-800/70 px-4 pb-4 pt-2"
                                >
                                  <div className="grid gap-4 sm:grid-cols-2">
                                    {Array.from({ length: end - start }).map((__, index) => {
                                      const seedIndex = start + index;
                                      return (
                                        <div
                                          key={`seed-${seedIndex}`}
                                          className="flex flex-col gap-2"
                                        >
                                          <label className="text-xs font-semibold text-slate-200/70">
                                            Seed {seedIndex + 1}
                                          </label>
                                          <input
                                            type="text"
                                            value={entries[seedIndex]}
                                            onChange={(e) =>
                                              handleEntryChange(seedIndex, e.target.value)
                                            }
                                            className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                                            placeholder={`Contender ${seedIndex + 1}`}
                                          />
                                          {seedInputError(seedIndex) && (
                                            <p className="text-xs text-rose-300">
                                              Contender name required.
                                            </p>
                                          )}
                                        </div>
                                      );
                                    })}
                                  </div>
                                </motion.div>
                              )}
                            </AnimatePresence>
                          </div>
                        );
                      })}
                    </div>
                  )}

                  {!hasMinimumSeeds && attemptedSubmit && (
                    <p className="text-sm text-rose-300">
                      You need at least two contenders to continue.
                    </p>
                  )}
                  {hasDuplicates && (
                    <p className="text-sm text-rose-300">
                      Duplicate contenders detected: {duplicateEntries.join(", ")}.
                    </p>
                  )}
                </motion.div>
              )}

              {step === 3 && (
                <motion.div
                  key="step-3"
                  initial={{ opacity: 0, x: -12 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 12 }}
                  transition={{ duration: 0.35 }}
                  className="flex flex-col gap-6"
                >
                  <div>
                    <h2 className="text-2xl font-semibold text-slate-50">Step 3 - Preview</h2>
                    <p className="mt-2 text-sm text-slate-200/70">
                      Watch the bracket structure update live. Adjust any seed before you confirm.
                    </p>
                  </div>
                  <div className="rounded-2xl border border-slate-700/60 bg-slate-950/40 p-4">
                    <div className="flex items-start gap-3 text-sm text-slate-200/70">
                      <FiCheckCircle className="text-emerald-300" />
                      <div>
                        <p className="font-semibold text-slate-100">Preview checklist</p>
                        <p className="mt-1">
                          Ensure your seeds are filled and the matchup pairings feel right.
                        </p>
                      </div>
                    </div>
                    {missingSeeds.length > 0 && (
                      <p className="mt-4 text-xs text-rose-300">
                        {missingSeeds.length} seed{missingSeeds.length > 1 ? "s" : ""} still
                        empty. Fill them before you publish.
                      </p>
                    )}
                  </div>
                </motion.div>
              )}

              {step === 4 && (
                <motion.div
                  key="step-4"
                  initial={{ opacity: 0, x: -12 }}
                  animate={{ opacity: 1, x: 0 }}
                  exit={{ opacity: 0, x: 12 }}
                  transition={{ duration: 0.35 }}
                  className="flex flex-col gap-6"
                >
                  <div>
                    <h2 className="text-2xl font-semibold text-slate-50">Step 4 - Review</h2>
                    <p className="mt-2 text-sm text-slate-200/70">
                      Confirm everything looks right before you publish.
                    </p>
                  </div>
                  <div className="grid gap-4 rounded-2xl border border-slate-700/60 bg-slate-950/40 p-4 text-sm text-slate-200/80">
                    <div>
                      <p className="text-xs uppercase tracking-[0.2em] text-slate-400">Title</p>
                      <p className="mt-1 text-base font-semibold text-slate-100">
                        {title.trim() || "Missing title"}
                      </p>
                    </div>
                    <div>
                      <p className="text-xs uppercase tracking-[0.2em] text-slate-400">Description</p>
                      <p className="mt-1">{description.trim() || "Missing description"}</p>
                    </div>
                    <div className="flex flex-wrap gap-3 text-xs font-semibold text-slate-200/70">
                      <span className="rounded-full border border-slate-600/50 px-3 py-1">
                        {size} contenders
                      </span>
                      <span className="rounded-full border border-slate-600/50 px-3 py-1">
                        {advanceMode === "manual" ? "Manual advance" : "Timer advance"}
                      </span>
                      {mutualsOnly && (
                        <span className="rounded-full border border-amber-400/50 px-3 py-1 text-amber-200">
                          Mutuals only
                        </span>
                      )}
                    </div>
                    {!timerValid && (
                      <p className="text-xs text-rose-300">
                        Timer rounds must be between 1 minute and 24 hours.
                      </p>
                    )}
                    {missingSeeds.length > 0 && (
                      <p className="text-xs text-rose-300">
                        Missing seeds: {missingSeeds.slice(0, 8).join(", ")}
                        {missingSeeds.length > 8 ? "..." : ""}.
                      </p>
                    )}
                    {hasDuplicates && (
                      <p className="text-xs text-rose-300">
                        Duplicate contenders found: {duplicateEntries.join(", ")}.
                      </p>
                    )}
                  </div>

                  {error && <p className="text-sm text-rose-300">{error}</p>}
                  {successMessage && (
                    <p className="text-sm text-emerald-300">{successMessage}</p>
                  )}
                </motion.div>
              )}
            </AnimatePresence>

            <div className="mt-8 flex flex-wrap items-center justify-between gap-3">
              <div className="flex gap-3">
                {step > 1 && (
                  <Button
                    type="button"
                    onClick={prevStep}
                    className="rounded-full border border-slate-600/60 bg-transparent px-5 py-2 text-sm font-semibold text-slate-100 hover:border-slate-400/70"
                  >
                    Back
                  </Button>
                )}
              </div>
              <div className="flex flex-col items-end gap-2 text-right">
                {step < 4 && (
                  <>
                    <Button
                      type="button"
                      onClick={nextStep}
                      className="rounded-full bg-sky-500/90 px-6 py-2 text-sm font-semibold text-slate-950 hover:bg-sky-400"
                    >
                      Continue
                    </Button>
                    {step === 1 && !basicsReady && attemptedSubmit && (
                      <span className="text-xs text-rose-300">
                        Add title and description to continue.
                      </span>
                    )}
                    {step === 2 && !hasMinimumSeeds && attemptedSubmit && (
                      <span className="text-xs text-rose-300">
                        You need at least two contenders to continue.
                      </span>
                    )}
                  </>
                )}
                {step === 4 && (
                  <>
                    <Button
                      type="submit"
                      disabled={!canCreate}
                      className={`rounded-full px-6 py-2 text-sm font-semibold transition ${
                        canCreate
                          ? "bg-amber-400 text-slate-950 hover:bg-amber-300"
                          : "border border-slate-600/60 bg-slate-900/40 text-slate-400"
                      }`}
                    >
                      {isSubmitting ? "Creating..." : "Create bracket"}
                    </Button>
                    {!canCreate && (
                      <span className="text-xs text-slate-300/70">
                        {isSubmitting
                          ? "Creating bracket..."
                          : !authedUserId
                            ? "Please sign in again to create a bracket."
                            : !basicsReady
                              ? "Add a title and description first."
                              : !allSeedsFilled
                                ? `Fill all ${size} contenders before publishing.`
                                : hasDuplicates
                                  ? "Remove duplicate contenders to continue."
                                  : !timerValid
                                    ? "Timer rounds must be between 1 minute and 24 hours."
                                    : ""}
                      </span>
                    )}
                  </>
                )}
              </div>
            </div>
          </motion.form>

          <div className="min-w-0 flex flex-col gap-4">
            <div className="flex items-center justify-between gap-3 rounded-2xl border border-slate-700/60 bg-slate-900/50 px-4 py-3 text-sm text-slate-200 lg:hidden">
              <span className="font-semibold">Preview panel</span>
              <Button
                type="button"
                onClick={() => setShowPreview((prev) => !prev)}
                className="rounded-full border border-slate-600/60 bg-transparent px-4 py-1 text-xs font-semibold text-slate-100"
              >
                {showPreview ? "Hide preview" : "Show preview"}
              </Button>
            </div>
            <AnimatePresence>{showPreview && previewCard}</AnimatePresence>

            <motion.div
              className="rounded-3xl border border-slate-700/60 bg-slate-900/70 p-6 shadow-[0_24px_60px_rgba(15,23,42,0.35)]"
              layout
            >
              <button
                type="button"
                onClick={() => setAdvancedOpen((prev) => !prev)}
                className="flex w-full items-center justify-between text-left text-sm font-semibold text-slate-100"
              >
                Advanced settings
                {advancedOpen ? <FiChevronDown /> : <FiChevronRight />}
              </button>
              <AnimatePresence>
                {advancedOpen && (
                  <motion.div
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    transition={{ duration: 0.3 }}
                    className="mt-4 flex flex-col gap-4 overflow-hidden"
                  >
                    <label className="flex items-center gap-3 text-sm text-slate-200">
                      <input
                        type="checkbox"
                        checked={mutualsOnly}
                        onChange={(e) => setMutualsOnly(e.target.checked)}
                        className="h-4 w-4 accent-sky-400"
                      />
                      Mutuals only
                    </label>
                    {advanceMode === "timer" && (
                      <div className="grid gap-3 sm:grid-cols-2">
                        <label className="flex flex-col gap-2 text-sm text-slate-200">
                          Round minutes
                          <input
                            type="number"
                            min="0"
                            value={roundMinutes}
                            onChange={(e) => setRoundMinutes(e.target.value)}
                            className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                            placeholder="60"
                          />
                        </label>
                        <label className="flex flex-col gap-2 text-sm text-slate-200">
                          Round seconds
                          <input
                            type="number"
                            min="0"
                            max="59"
                            value={roundSeconds}
                            onChange={(e) => setRoundSeconds(e.target.value)}
                            className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                            placeholder="0"
                          />
                        </label>
                      </div>
                    )}
                    <label className="flex flex-col gap-2 text-sm text-slate-200">
                      Tags or categories (optional)
                      <input
                        type="text"
                        value={tags}
                        onChange={(e) => setTags(e.target.value)}
                        className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                        placeholder="Music, sports, pop culture"
                      />
                    </label>
                    <label className="flex flex-col gap-2 text-sm text-slate-200">
                      Notes or rules (optional)
                      <textarea
                        rows={3}
                        value={notes}
                        onChange={(e) => setNotes(e.target.value)}
                        className="rounded-xl border border-slate-700/60 bg-slate-900/60 px-3 py-2 text-sm text-slate-100 outline-none transition focus:border-sky-400/70 focus:ring-2 focus:ring-sky-400/20"
                        placeholder="Any rules you want contenders to follow."
                      />
                    </label>
                  </motion.div>
                )}
              </AnimatePresence>
            </motion.div>
          </div>
        </div>
      </main>
    </div>
  );
};

export default CreateBracketPage;
