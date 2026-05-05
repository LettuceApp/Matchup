import { useCallback, useEffect, useState } from 'react';
import { getAnonVoteStatus } from '../services/api';
import { peekAnonId } from '../utils/anonId';

/*
 * useAnonVoteStatus — single source of truth for the per-browser
 * vote counter shown to anonymous users.
 *
 * Fetches `{used, max}` from the server on mount IF an anon UUID
 * already exists. If no UUID is set yet (the user hasn't voted), we
 * default to `{used: 0, max: 3}` locally — saves a round-trip on
 * every home-page mount for visitors who'll never vote.
 *
 * Returns:
 *   { used, max, remaining, atCap, refresh, bumpOptimistic }
 *
 * Design notes:
 *   * `refresh()` re-pulls from the server. Call after a successful
 *     vote so the UI eventually agrees with the server's count even
 *     if the optimistic bump was wrong (e.g. duplicate vote).
 *   * `bumpOptimistic()` is the immediate "I just voted" path —
 *     increments `used` locally so the counter chip updates without
 *     waiting for the round-trip. Followed by `refresh()` for
 *     correctness.
 *   * Skips entirely for authed users: if there's a token,
 *     consumers are expected to NOT mount this hook. The hook
 *     does NOT defensive-check the token; that's the caller's job.
 *
 * The hook short-circuits for SSR / non-browser env (no localStorage)
 * by leaving `used` at 0 and `loading` at false.
 */
export const useAnonVoteStatus = () => {
  const [used, setUsed] = useState(0);
  const [max, setMax] = useState(3);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    if (typeof window === 'undefined') return;
    if (!peekAnonId()) {
      // No anon UUID yet → no votes to count. Don't burn a round-trip.
      setUsed(0);
      return;
    }
    setLoading(true);
    try {
      const res = await getAnonVoteStatus();
      const payload = res?.data?.response || res?.data || {};
      setUsed(Number(payload.used ?? 0));
      setMax(Number(payload.max ?? 3));
    } catch (err) {
      // Stale local state is preferable to a broken UI; the next
      // vote attempt will sync server-side anyway.
      console.warn('getAnonVoteStatus failed', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const bumpOptimistic = useCallback(() => {
    setUsed((prev) => Math.min(prev + 1, max));
  }, [max]);

  const remaining = Math.max(max - used, 0);
  const atCap = remaining <= 0;

  return { used, max, remaining, atCap, loading, refresh, bumpOptimistic };
};
