import { Navigate, useLocation } from 'react-router-dom';

function hasToken() {
  return Boolean(localStorage.getItem('token'));
}

export function RequireAuth({ children }) {
  const location = useLocation();
  return hasToken() ? (
    children
  ) : (
    <Navigate to="/login" replace state={{ from: location }} />
  );
}

export function RedirectIfAuth({ children }) {
  return hasToken() ? <Navigate to="/home" replace /> : children;
}

export function RequireAdmin({ children }) {
  const location = useLocation();
  const isAdmin = localStorage.getItem('isAdmin') === 'true';
  return isAdmin ? (
    children
  ) : (
    <Navigate to="/home" replace state={{ from: location }} />
  );
}
