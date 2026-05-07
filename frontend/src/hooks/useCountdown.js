import { useCallback, useEffect, useState } from "react";

export default function useCountdown(targetTime) {
  // Wrapped in useCallback so the function identity is stable across
  // renders for the same targetTime — that lets the effect below depend
  // on it without re-firing every render. (Was previously not memoized,
  // so eslint flagged the effect for missing deps.)
  const getRemaining = useCallback(() => {
    if (!targetTime) return null;
    const diff = new Date(targetTime).getTime() - Date.now();
    return Math.max(0, diff);
  }, [targetTime]);

  const [remainingMs, setRemainingMs] = useState(getRemaining);

  useEffect(() => {
    if (!targetTime) {
      setRemainingMs(null);
      return;
    }

    setRemainingMs(getRemaining());

    const interval = setInterval(() => {
      setRemainingMs(getRemaining());
    }, 1000);

    return () => clearInterval(interval);
  }, [targetTime, getRemaining]);

  const isExpired = remainingMs !== null && remainingMs <= 0;

  const safeRemaining = remainingMs ?? 0;
  const seconds = Math.floor((safeRemaining / 1000) % 60);
  const minutes = Math.floor((safeRemaining / (1000 * 60)) % 60);
  const hours = Math.floor(safeRemaining / (1000 * 60 * 60));

  return {
    remainingMs,
    isExpired,
    formatted: `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`,
  };
}
