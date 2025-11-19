// useAuthBootstrap.js
import { useEffect, useState } from 'react';
import API, { getCurrentUser } from '../services/api'; // <-- default + named

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
        const payload = res?.data?.response || res?.data;
        if (payload?.id) localStorage.setItem('userId', String(payload.id));
        if (typeof payload?.is_admin === 'boolean') {
          localStorage.setItem('isAdmin', payload.is_admin ? 'true' : 'false');
        }
      })
      .catch((err) => {
        if (!mounted) return;
        // only wipe token on 401 (don’t log out for 404/network)
        if (err?.response?.status === 401) {
          localStorage.removeItem('token');
          localStorage.removeItem('userId');
          localStorage.removeItem('isAdmin');
          delete API.defaults.headers.common.Authorization;
        }
      })
      .finally(() => mounted && setReady(true));

    return () => { mounted = false };
  }, []);

  return ready;
}
