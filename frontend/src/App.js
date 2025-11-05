import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import HomePage from './pages/HomePage';
import LoginPage from './pages/LoginPage';
import RegisterPage from './pages/RegisterPage';
import MatchupPage from './pages/MatchupPage';
import CreateMatchup from './pages/CreateMatchup';
import UserProfile from './pages/UserProfile';
import { RequireAuth, RedirectIfAuth } from './auth/guards';
import { useAuthBootstrap } from './auth/useAuthBootstrap';

function App() {
  const ready = useAuthBootstrap();

  if (!ready) {
    return null;
  }

  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/"
          element={
            <RequireAuth>
              <HomePage />
            </RequireAuth>
          }
        />
        <Route
          path="/home"
          element={
            <RequireAuth>
              <HomePage />
            </RequireAuth>
          }
        />
        <Route
          path="/login"
          element={
            <RedirectIfAuth>
              <LoginPage />
            </RedirectIfAuth>
          }
        />
        <Route
          path="/register"
          element={
            <RedirectIfAuth>
              <RegisterPage />
            </RedirectIfAuth>
          }
        />
        <Route
          path="/users/:uid/matchup/:id"
          element={
            <RequireAuth>
              <MatchupPage />
            </RequireAuth>
          }
        />
        <Route
          path="/users/:userId/create-matchup"
          element={
            <RequireAuth>
              <CreateMatchup />
            </RequireAuth>
          }
        />
        <Route
          path="/users/:userId/profile"
          element={
            <RequireAuth>
              <UserProfile />
            </RequireAuth>
          }
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
