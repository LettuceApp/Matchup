import { lazy, Suspense } from 'react';
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
} from 'react-router-dom';
import { AnimatePresence } from 'framer-motion';
// Eager — these are reachable on first paint and would just delay the spinner
// if lazy-loaded. HomePage is the default landing route; Login/Register are
// the unauth landing; UserProfileRedirect is a tiny redirect-only stub.
import HomePage from './pages/HomePage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import UserProfileRedirect from './pages/UserProfileRedirect';
import { RequireAuth, RedirectIfAuth, RequireAdmin } from './auth/guards';
import { useAuthBootstrap } from './auth/useAuthBootstrap';
import PageTransition from './components/PageTransition';

// Lazy — these import heavy libraries (react-tournament-bracket, recharts,
// markdown editors) that the home page user may never reach. Each becomes
// its own webpack chunk and is downloaded on first navigation to that route.
const MatchupPage = lazy(() => import('./pages/MatchupPage'));
const CreateMatchup = lazy(() => import('./pages/CreateMatchup'));
const UserProfile = lazy(() => import('./pages/UserProfile'));
const AdminDashboard = lazy(() => import('./pages/AdminDashboard'));
const CreateBracketPage = lazy(() => import('./pages/CreateBracketPage'));
const BracketPage = lazy(() => import('./pages/BracketPage'));

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

  return (
    <AnimatePresence mode="wait">
      <Suspense fallback={<RouteFallback />}>
      <Routes location={location} key={location.pathname}>
        <Route
          path="/"
          element={
            <RequireAuth>
              <PageTransition>
                <HomePage />
              </PageTransition>
            </RequireAuth>
          }
        />
        <Route
          path="/home"
          element={
            <RequireAuth>
              <PageTransition>
                <HomePage />
              </PageTransition>
            </RequireAuth>
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
        <Route
          path="/users/:userId/profile"
          element={
            <RequireAuth>
              <PageTransition>
                <UserProfileRedirect />
              </PageTransition>
            </RequireAuth>
          }
        />
        <Route
          path="/users/:username"
          element={
            <RequireAuth>
              <PageTransition>
                <UserProfile />
              </PageTransition>
            </RequireAuth>
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
            <PageTransition>
              <BracketPage />
            </PageTransition>
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
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
      </Suspense>
    </AnimatePresence>
  );
};

function App() {
  const ready = useAuthBootstrap();

  if (!ready) {
    return null;
  }

  return (
    <BrowserRouter>
      <AppRoutes />
    </BrowserRouter>
  );
}

export default App;
