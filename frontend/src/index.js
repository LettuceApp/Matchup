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
import './index.css';
import App from './App';
import reportWebVitals from './reportWebVitals';

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
