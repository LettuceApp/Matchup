/*
 * anonId — per-browser UUID for anonymous voters.
 *
 * Lifecycle:
 *   1. Anonymous user opens the app, browses matchups. NO anon ID is
 *      generated yet — we don't pollute localStorage on read.
 *   2. They click vote on their first matchup. `getOrCreateAnonId()`
 *      fires, mints a fresh UUID, persists it, and the vote request
 *      includes it as `anon_id`.
 *   3. Subsequent votes reuse the same ID. Server enforces the 3-vote
 *      cap by counting matchup_votes rows with that anon_id.
 *   4. User signs up. The signup request sends X-Anon-Id; the server
 *      runs the existing mergeDeviceVotesToUser to migrate their
 *      anon votes into the new account. Frontend then calls
 *      `clearAnonId()` so the browser doesn't keep an orphan ID.
 *
 * Why lazy generation: a stable UUID set on first page load shows up
 * in Chrome DevTools' Application tab, which an attentive privacy-
 * aware user would notice. Generating only when there's a real anon
 * vote keeps the storage clean for the 90% of visitors who never
 * cast one.
 *
 * crypto.randomUUID() is available everywhere modern (Chrome 92+,
 * Safari 15.4+, Firefox 95+). The jsdom test env Polyfills it from
 * Node 19's stdlib.
 */

const STORAGE_KEY = 'anonId';

// Read-only check — used by the axios interceptor to decide whether
// to attach the X-Anon-Id header. Never generates as a side effect
// of a request; only the explicit vote handlers should mint an ID.
export const peekAnonId = () => {
  try {
    return typeof localStorage !== 'undefined'
      ? localStorage.getItem(STORAGE_KEY)
      : null;
  } catch {
    // localStorage can throw in private-browsing modes / iframe
    // sandboxes. Return null so callers fall back gracefully.
    return null;
  }
};

// Returns the existing anon UUID, or mints + persists a fresh one.
// Call from the vote handler RIGHT BEFORE sending the request.
export const getOrCreateAnonId = () => {
  const existing = peekAnonId();
  if (existing) return existing;

  // Browsers + jsdom both expose crypto.randomUUID. Fall back to a
  // simple random string just in case.
  let next;
  try {
    next = (typeof crypto !== 'undefined' && crypto.randomUUID)
      ? crypto.randomUUID()
      : Math.random().toString(36).slice(2) + Date.now().toString(36);
  } catch {
    next = Math.random().toString(36).slice(2) + Date.now().toString(36);
  }

  try {
    localStorage.setItem(STORAGE_KEY, next);
  } catch {
    // Persist failure — return the value anyway so this single
    // request still works; future requests will mint a fresh ID
    // each time, which is fine for cap purposes (server still
    // counts each unique anon ID independently).
  }
  return next;
};

// Called after a successful signup so the new account doesn't keep
// dragging the old anon ID around. The server has already migrated
// the votes into the user account; the anon ID is dead weight after
// that.
export const clearAnonId = () => {
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* swallow */
  }
};
