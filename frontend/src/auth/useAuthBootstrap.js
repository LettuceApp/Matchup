// useAuthBootstrap.js
import { useEffect, useState } from 'react';
import API, { getCurrentUser, signOutLocally } from '../services/api'; // <-- default + named
import { identifyUser } from '../utils/analytics';

export function useAuthBootstrap() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let mounted = true;
    const token = localStorage.getItem('token');

    if (!token) {
      localStorage.removeItem('isAdmin');
      setReady(true);
      return () => { mounted = false };
    }

    // Reattach Authorization BEFORE calling the API
    API.defaults.headers.common.Authorization = `Bearer ${token}`;

    getCurrentUser()
      .then((res) => {
        if (!mounted) return;
        const payload = res?.data?.user || res?.data?.response || res?.data;
        if (payload?.id) localStorage.setItem('userId', String(payload.id));
        if (payload?.username) localStorage.setItem('username', String(payload.username));
        if (typeof payload?.is_admin === 'boolean') {
          localStorage.setItem('isAdmin', payload.is_admin ? 'true' : 'false');
        }
        // Identify the user to PostHog so every event from this load
        // onward carries cohort attributes. No-op when PostHog wasn't
        // initialised (dev / no key configured).
        if (payload?.id) {
          identifyUser(payload.id, {
            username: payload.username,
            is_admin: Boolean(payload.is_admin),
            is_verified: Boolean(payload.is_verified),
          });
        }
      })
      .catch((err) => {
        if (!mounted) return;
        // only wipe token on 401 (don’t log out for 404/network).
        // Reached only after the refresh interceptor in api.js has
        // already tried + failed to rotate — treat as terminal.
        if (err?.response?.status === 401) {
          signOutLocally();
        }
      })
      .finally(() => mounted && setReady(true));

    return () => { mounted = false };
  }, []);

  return ready;
}
