import React from 'react';
import ReactDOM from 'react-dom/client';
// Side-effect imports — initialise observability SDKs before any
// other module mounts. Order matters: Sentry first so its
// ErrorBoundary in App.js sees a ready SDK; PostHog right after so
// the autocapture pageview for the initial route fires. Both modules
// no-op cleanly when their respective env vars are unset (dev
// laptops + tests), so this is safe to import unconditionally.
import './sentry';
import './posthog';
// Theme tokens must land BEFORE any component stylesheet runs so
// the var(--…) references resolve on first paint. theme.css ships
// the dark palette under both :root and [data-theme="dark"], so
// the page is fully styled even before bootstrapTheme() flips the
// data-theme attribute on <html>.
import './styles/theme.css';
import './index.css';
import { bootstrapTheme } from './utils/theme';
import App from './App';
import reportWebVitals from './reportWebVitals';

// Apply the user's stored theme (or OS preference) to <html>
// BEFORE React mounts, so light-mode users don't see a dark flash
// during the first render.
bootstrapTheme();

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
