import * as Sentry from '@sentry/react';

/*
 * Frontend Sentry wrapper.
 *
 * Initialised once at module-load (side-effect import from index.js)
 * so the `Sentry.ErrorBoundary` in App.js and the interceptor hook
 * in services/api.js both see a ready SDK.
 *
 * All env vars come through Create React App's process.env pipeline
 * and must therefore be prefixed REACT_APP_*. Empty DSN → no-op
 * (we skip init entirely so the dev console isn't polluted with
 * "Sentry client not initialised" warnings).
 *
 * Config notes:
 *   tracesSampleRate: 1.0 in staging, 0.1 in prod — same logic the
 *     backend uses. We keep it high in staging because the traffic
 *     volume is low and the sample rate matters more than cost.
 *   replaysSessionSampleRate: 0 — Session Replay can spike storage
 *     costs hard on image-heavy matchup pages. We enable it only
 *     for error traces (replaysOnErrorSampleRate) so the replay
 *     is attached to things we need to reproduce, nothing else.
 */

const dsn = process.env.REACT_APP_SENTRY_DSN;
const environment = process.env.REACT_APP_SENTRY_ENVIRONMENT || process.env.NODE_ENV || 'development';
const release = process.env.REACT_APP_SENTRY_RELEASE || undefined;

if (dsn) {
  Sentry.init({
    dsn,
    environment,
    release,
    // Performance + Replay integrations are auto-registered by the
    // default integrations list in @sentry/react v10. No manual
    // `integrations: [...]` needed unless we want to customise.
    tracesSampleRate: environment === 'production' ? 0.1 : 1.0,
    replaysSessionSampleRate: 0,
    replaysOnErrorSampleRate: 0.1,
    // Strip noisy errors that aren't actionable. These show up from
    // third-party scripts (ad blockers, browser extensions) and
    // would otherwise burn through the free-tier event quota.
    ignoreErrors: [
      'ResizeObserver loop limit exceeded',
      'Non-Error promise rejection captured',
    ],
  });
  // eslint-disable-next-line no-console
  console.log(`[sentry] initialised env=${environment} release=${release || 'unset'}`);
}

export { Sentry };
