import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useLocation,
} from 'react-router-dom';
import { AnimatePresence } from 'framer-motion';
import HomePage from './pages/HomePage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import MatchupPage from './pages/MatchupPage';
import CreateMatchup from './pages/CreateMatchup';
import UserProfile from './pages/UserProfile';
import UserProfileRedirect from './pages/UserProfileRedirect';
import AdminDashboard from './pages/AdminDashboard';
import CreateBracketPage from './pages/CreateBracketPage';
import BracketPage from './pages/BracketPage';
import { RequireAuth, RedirectIfAuth, RequireAdmin } from './auth/guards';
import { useAuthBootstrap } from './auth/useAuthBootstrap';
import PageTransition from './components/PageTransition';

const AppRoutes = () => {
  const location = useLocation();

  return (
    <AnimatePresence mode="wait">
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
