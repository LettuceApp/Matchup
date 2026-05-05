import posthogJs from 'posthog-js';

/*
 * Frontend PostHog wrapper — product-analytics counterpart to sentry.js.
 *
 * Bootstrapped once at module-load via a side-effect import from
 * index.js. Empty REACT_APP_POSTHOG_KEY → init skipped entirely (no
 * console noise in dev, no events leaked from local laptops).
 *
 * What this enables:
 *   * Autocapture for pageviews + DOM clicks (zero per-callsite work)
 *   * Explicit funnel events via the `track(...)` helper in
 *     utils/analytics.js — see that file for the per-event payload
 *     shape that auto-attaches { is_anon, anon_id, user_id }.
 *   * Anon → user identity merge via posthog.identify(userId, ...)
 *     called from RegisterPage / LoginPage / useAuthBootstrap.
 *
 * Config rationale:
 *   * persistence: 'localStorage' — no third-party cookies, lighter
 *     privacy story, no consent banner required for v1.
 *   * respect_dnt: true — users sending Do Not Track aren't tracked.
 *   * capture_pageview: true — covers SPA route changes via the
 *     PostHog history-listener, no manual hook in App.js needed.
 *   * autocapture: true — picks up the long tail of click events
 *     beyond the 13 explicit funnel events we ship in this cycle.
 *
 * The exported `posthog` object is a no-op stub when init was
 * skipped, so callers (analytics.js, identify sites) don't need
 * defensive null checks.
 */

const key = process.env.REACT_APP_POSTHOG_KEY;
const apiHost = process.env.REACT_APP_POSTHOG_HOST || 'https://us.i.posthog.com';
const environment =
  process.env.REACT_APP_POSTHOG_ENVIRONMENT ||
  process.env.NODE_ENV ||
  'development';

let initialised = false;

if (key) {
  posthogJs.init(key, {
    api_host: apiHost,
    persistence: 'localStorage',
    autocapture: true,
    capture_pageview: true,
    respect_dnt: true,
    // Tag every event with the deploy environment as a super-property
    // so PostHog can split prod / staging / dev without separate keys.
    loaded: (ph) => {
      ph.register({ environment });
      // Expose for browser-console debugging in dev only. Avoid
      // window-pollution in prod where it would also surface in
      // attacker reconnaissance scans.
      if (environment !== 'production') {
        // eslint-disable-next-line no-undef
        window.__posthog = ph;
      }
    },
  });
  initialised = true;
  // eslint-disable-next-line no-console
  console.log(`[posthog] initialised env=${environment}`);
}

// No-op stub for code paths that run before init OR when key is
// empty. Same shape as the real SDK for the methods we call across
// the app — capture, identify, register, reset. Adding a method here
// later costs nothing if the codebase grows new callsites.
const noopStub = {
  capture: () => {},
  identify: () => {},
  register: () => {},
  reset: () => {},
  isFeatureEnabled: () => false,
  getDistinctId: () => undefined,
};

export const posthog = initialised ? posthogJs : noopStub;
export const isPosthogInitialised = () => initialised;
