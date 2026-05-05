import { posthog } from '../posthog';

/*
 * track — single chokepoint for explicit funnel + engagement events.
 *
 * Every event automatically carries:
 *   - is_anon:  true when there's no auth token in localStorage
 *   - anon_id:  the per-browser UUID (when present)
 *   - user_id:  the signed-in user's id (when present)
 *
 * Adding the same three properties at the call site every time would
 * be 39 extra lines of duplication across 13 events. Centralising
 * them here makes each callsite a one-liner like:
 *
 *   import { track } from '../utils/analytics';
 *   track('vote_cast', { matchup_id: m.id, is_bracket: false });
 *
 * The PostHog SDK is a no-op stub (see posthog.js) when init was
 * skipped, so the function is safe to call before bootstrap or in
 * test environments without a key.
 */
export const track = (event, props = {}) => {
  // Read identity from localStorage at call time — the values can
  // shift between init and the first event (e.g., user logs in
  // mid-session). Reading on every call keeps the cohort attribution
  // honest at the cost of two trivial localStorage reads per event.
  let isAnon = true;
  let anonId;
  let userId;
  try {
    if (typeof localStorage !== 'undefined') {
      isAnon = !localStorage.getItem('token');
      anonId = localStorage.getItem('anonId') || undefined;
      userId = localStorage.getItem('userId') || undefined;
    }
  } catch {
    // SSR / sandboxed iframe / private mode — keep defaults.
  }

  posthog.capture(event, {
    is_anon: isAnon,
    anon_id: anonId,
    user_id: userId,
    ...props,
  });
};

/*
 * identifyUser — alias for posthog.identify with a guard so callers
 * don't need to import the SDK directly. Pass the user's primary id
 * as the distinctId so anon events (under the anon UUID) get aliased
 * into the user's identity graph the next time PostHog processes the
 * events.
 */
export const identifyUser = (userId, props = {}) => {
  if (!userId) return;
  posthog.identify(String(userId), props);
};

/*
 * resetIdentity — call on full sign-out so the next session doesn't
 * keep accumulating events under the previous user's distinctId.
 * No-op when SDK isn't initialised.
 */
export const resetIdentity = () => {
  posthog.reset();
};
