import { lazy, Suspense, useEffect } from 'react';
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
} from 'react-router-dom';
import { AnimatePresence } from 'framer-motion';
import { track } from './utils/analytics';
// Eager — these are reachable on first paint and would just delay the spinner
// if lazy-loaded. HomePage is the default landing route; Login/Register are
// the unauth landing; UserProfileRedirect is a tiny redirect-only stub.
import HomePage from './pages/HomePage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import PrivacyPage from './pages/PrivacyPage';
import TermsPage from './pages/TermsPage';
import UserProfileRedirect from './pages/UserProfileRedirect';
import { RequireAuth, RedirectIfAuth, RequireAdmin } from './auth/guards';
import { useAuthBootstrap } from './auth/useAuthBootstrap';
import PageTransition from './components/PageTransition';
import EmailVerificationBanner from './components/EmailVerificationBanner';
import { AnonUpgradeProvider } from './contexts/AnonUpgradeContext';
import { Sentry } from './sentry';

// Lazy — these import heavy libraries (react-tournament-bracket, recharts,
// markdown editors) that the home page user may never reach. Each becomes
// its own webpack chunk and is downloaded on first navigation to that route.
const MatchupPage = lazy(() => import('./pages/MatchupPage'));
const CreateMatchup = lazy(() => import('./pages/CreateMatchup'));
const UserProfile = lazy(() => import('./pages/UserProfile'));
const AdminDashboard = lazy(() => import('./pages/AdminDashboard'));
const CreateBracketPage = lazy(() => import('./pages/CreateBracketPage'));
const BracketPage = lazy(() => import('./pages/BracketPage'));
const AccountSettings = lazy(() => import('./pages/AccountSettings'));
const BlocksAndMutes = lazy(() => import('./pages/BlocksAndMutes'));
const ForgotPasswordPage = lazy(() => import('./pages/ForgotPasswordPage'));
const ResetPasswordPage = lazy(() => import('./pages/ResetPasswordPage'));
const VerifyEmailPage = lazy(() => import('./pages/VerifyEmailPage'));

// Tiny inline fallback shown while a lazy chunk is downloading. Kept inline
// rather than importing a heavyweight spinner so the fallback itself never
// triggers another chunk load.
const RouteFallback = () => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      minHeight: '60vh',
      color: '#888',
      fontSize: 14,
    }}
  >
    Loading…
  </div>
);

const AppRoutes = () => {
  const location = useLocation();

  // Push-notification CTR. The service worker rewrites click URLs to
  // include ?utm_source=push (+ push_kind from the notification tag).
  // This effect detects that on landing, fires the funnel event, and
  // strips the query so a refresh doesn't double-count.
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (params.get('utm_source') !== 'push') return;
    const pushKind = params.get('push_kind') || undefined;
    track('push_clicked', {
      target_path: location.pathname,
      push_kind: pushKind,
    });
    // Replace history without the utm params so it doesn't pollute
    // shares + so a refresh doesn't re-fire the event.
    const cleanURL = location.pathname + location.hash;
    window.history.replaceState({}, '', cleanURL);
    // Run only when the path or query changes — and only the first
    // time after a push-click landing.
  }, [location.pathname, location.search, location.hash]);

  return (
    <AnimatePresence mode="wait">
      {/* Email-verification nudge for signed-in, unverified users.
          Mounted outside the per-route Suspense boundary so it
          persists across navigations; the component self-gates when
          there's no user or the user's already verified. */}
      <EmailVerificationBanner />
      <Suspense fallback={<RouteFallback />}>
      <Routes location={location} key={location.pathname}>
        {/* HomePage is anon-friendly. Anonymous visitors get the
            popular feed, can vote on up to 3 standalone matchups,
            and see the Sign up CTA in the navbar. RequireAuth is
            still enforced on profile / settings / create-matchup
            below — the anon UX intentionally stops at "browse +
            vote a few times". */}
        <Route
          path="/"
          element={
            <PageTransition>
              <HomePage />
            </PageTransition>
          }
        />
        <Route
          path="/home"
          element={
            <PageTransition>
              <HomePage />
            </PageTransition>
          }
        />
        <Route
          path="/login"
          element={
            <RedirectIfAuth>
              <PageTransition>
                <LoginPage />
              </PageTransition>
            </RedirectIfAuth>
          }
        />
        <Route
          path="/register"
          element={
            <RedirectIfAuth>
              <PageTransition>
                <RegisterPage />
              </PageTransition>
            </RedirectIfAuth>
          }
        />
        {/* Forgot-password flow. Both routes are public because the
            user is by definition locked out — guarding behind auth
            would create a catch-22. /reset-password/:token is also
            listed in the AASA `exclude` set so native-app dispatch
            doesn't swallow the link. */}
        <Route
          path="/forgot-password"
          element={
            <PageTransition>
              <ForgotPasswordPage />
            </PageTransition>
          }
        />
        <Route
          path="/reset-password/:token"
          element={
            <PageTransition>
              <ResetPasswordPage />
            </PageTransition>
          }
        />
        {/* Email verification link target. Public because the user
            might click it from a different device than the one where
            they signed up — we can't assume a valid session. The
            handler is anonymous (token-only) by design. Listed in the
            AASA `exclude` set to keep it in the browser. */}
        <Route
          path="/verify-email/:token"
          element={
            <PageTransition>
              <VerifyEmailPage />
            </PageTransition>
          }
        />
        <Route
          path="/users/:uid/matchup/:id"
          element={
            <PageTransition>
              <MatchupPage />
            </PageTransition>
          }
        />
        <Route
          path="/users/:userId/create-matchup"
          element={
            <RequireAuth>
              <PageTransition>
                <CreateMatchup />
              </PageTransition>
            </RequireAuth>
          }
        />
        {/* Profile pages are anon-friendly. The backend's GetUser
            returns 404 when an anonymous viewer asks for a private
            profile — same shape as a missing user, no leak. The page
            itself self-gates the owner-only actions (Edit, Notification
            settings, Block menu) on the presence of a viewer. */}
        <Route
          path="/users/:userId/profile"
          element={
            <PageTransition>
              <UserProfileRedirect />
            </PageTransition>
          }
        />
        <Route
          path="/users/:username"
          element={
            <PageTransition>
              <UserProfile />
            </PageTransition>
          }
        />
        <Route
          path="/brackets/new"
          element={
            <RequireAuth>
              <PageTransition>
                <CreateBracketPage />
              </PageTransition>
            </RequireAuth>
          }
        />
        <Route
          path="/brackets/:id"
          element={
            <RequireAuth>
              <PageTransition>
                <BracketPage />
              </PageTransition>
            </RequireAuth>
          }
        />
        <Route
          path="/admin"
          element={
            <RequireAuth>
              <RequireAdmin>
                <PageTransition>
                  <AdminDashboard />
                </PageTransition>
              </RequireAdmin>
            </RequireAuth>
          }
        />
        {/* Auth-required account-management page. Delete-my-account
            is the current surface; future self-serve actions (change
            password / download data) land here too. */}
        <Route
          path="/settings/account"
          element={
            <RequireAuth>
              <PageTransition>
                <AccountSettings />
              </PageTransition>
            </RequireAuth>
          }
        />
        {/* Blocks + mutes — viewer-only surface listing every user the
            viewer has blocked or muted with a one-tap reverse action. */}
        <Route
          path="/settings/blocks"
          element={
            <RequireAuth>
              <PageTransition>
                <BlocksAndMutes />
              </PageTransition>
            </RequireAuth>
          }
        />
        {/* Legal pages — public, no auth gate. Linked from the
            LoginPage / RegisterPage footers and cited in the App Store /
            Play Store submission forms. Content is placeholder; see
            the banner on each page. */}
        <Route
          path="/privacy"
          element={
            <PageTransition>
              <PrivacyPage />
            </PageTransition>
          }
        />
        <Route
          path="/terms"
          element={
            <PageTransition>
              <TermsPage />
            </PageTransition>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      </Suspense>
    </AnimatePresence>
  );
};

/*
 * SentryFallback — rendered by Sentry.ErrorBoundary when a render-
 * path exception bubbles past every component-level handler. The
 * user gets a calm "something broke, we've been notified" screen + a
 * reload button. Session Replay (configured in sentry.js) captures
 * the last ~30s so we can reproduce without asking them for steps.
 */
const SentryFallback = () => (
  <div
    role="alert"
    style={{
      minHeight: '60vh',
      display: 'flex',
      flexDirection: 'column',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 24,
      gap: 16,
      color: '#e0e6ff',
      background: 'linear-gradient(180deg, #0b0f1c 0%, #0e1426 100%)',
      textAlign: 'center',
    }}
  >
    <h1 style={{ margin: 0, fontSize: '1.4rem' }}>Something broke</h1>
    <p style={{ margin: 0, maxWidth: 360, color: '#9aa2c4' }}>
      Our team has been notified — reloading usually gets things back on track.
    </p>
    <button
      type="button"
      onClick={() => window.location.reload()}
      style={{
        marginTop: 8,
        padding: '10px 24px',
        borderRadius: 999,
        border: '1px solid rgba(255,255,255,0.2)',
        background: 'rgba(255,255,255,0.06)',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '0.95rem',
      }}
    >
      Reload
    </button>
  </div>
);

function App() {
  const ready = useAuthBootstrap();

  if (!ready) {
    return null;
  }

  return (
    <Sentry.ErrorBoundary fallback={<SentryFallback />}>
      <BrowserRouter>
        {/* AnonUpgradeProvider mounts the global "sign up to keep
            voting" modal once at the app root. Any descendant can
            call useAnonUpgradePrompt().promptUpgrade('cap'|'bracket'
            |'like'|'comment') without prop-drilling. */}
        <AnonUpgradeProvider>
          <AppRoutes />
        </AnonUpgradeProvider>
      </BrowserRouter>
    </Sentry.ErrorBoundary>
  );
}

export default App;
