import { useEffect } from 'react';

/*
 * useShareTracking — attribution on incoming shared links.
 *
 * When a user opens `.../m/Kj3mN9xP?ref=tw`, we want to (a) know the
 * traffic came from Twitter, and (b) strip the `?ref=` so if they
 * re-share from the address bar they don't pollute attribution for the
 * next hop. We also stash the ref in localStorage so a later signup
 * can attribute back to the first share channel.
 *
 * Call once per page from the landing component (MatchupPage,
 * BracketPage). The beacon POST is fire-and-forget — failures are
 * intentionally ignored.
 */

const STORAGE_KEY = 'matchup.share_ref';
const VALID_REFS = new Set([
  'tw', 'fb', 'li', 'copy', 'native', 'email', 'sms', 'ig', 'dm', 'discord', 'slack',
]);

function readRef() {
  try {
    const params = new URLSearchParams(window.location.search);
    const ref = params.get('ref');
    if (!ref) return null;
    // Reject garbage — an attacker injecting arbitrary strings would
    // blow up our Prometheus label cardinality.
    if (!VALID_REFS.has(ref)) return null;
    return ref;
  } catch {
    return null;
  }
}

function stripRefFromURL() {
  try {
    const url = new URL(window.location.href);
    if (!url.searchParams.has('ref')) return;
    url.searchParams.delete('ref');
    const next = url.pathname + (url.searchParams.toString() ? '?' + url.searchParams.toString() : '') + url.hash;
    window.history.replaceState({}, '', next);
  } catch {
    // noop
  }
}

function storeRef(ref) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify({
      ref,
      at: Date.now(),
    }));
  } catch {
    // Private mode / storage full — not worth the error surface.
  }
}

function sendBeacon(ref, contentType, shortID) {
  try {
    const body = JSON.stringify({
      source: ref,
      content_type: contentType,
      short_id: shortID || '',
    });
    // sendBeacon is fire-and-forget by design and survives page unload
    // better than fetch — exactly what we want here.
    if (navigator.sendBeacon) {
      const blob = new Blob([body], { type: 'application/json' });
      navigator.sendBeacon('/share/landed', blob);
      return;
    }
    fetch('/share/landed', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body,
      keepalive: true,
    }).catch(() => { /* ignore */ });
  } catch {
    // noop
  }
}

/**
 * Call this once per landing on a share-capable page.
 *
 * @param {object} opts
 * @param {'matchup'|'bracket'} opts.contentType
 * @param {string} [opts.shortID] — 8-char id if known; falls through to empty
 */
export default function useShareTracking({ contentType, shortID } = {}) {
  useEffect(() => {
    if (typeof window === 'undefined') return;
    const ref = readRef();
    if (!ref) return;

    storeRef(ref);
    sendBeacon(ref, contentType, shortID);
    stripRefFromURL();
  }, [contentType, shortID]);
}

// Exported for signup-attribution call sites that want to read the
// last share ref without running the hook.
export function getStoredShareRef() {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const { ref, at } = JSON.parse(raw);
    // 30-day window — anything older is probably unrelated.
    if (!ref || Date.now() - at > 30 * 24 * 60 * 60 * 1000) return null;
    return ref;
  } catch {
    return null;
  }
}
