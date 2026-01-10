import { useEffect, useState } from "react";

export default function useCountdown(targetTime) {
  const getRemaining = () => {
    if (!targetTime) return null;
    const diff = new Date(targetTime).getTime() - Date.now();
    return Math.max(0, diff);
  };

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
  }, [targetTime]);

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
