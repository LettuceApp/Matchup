import { useEffect, useState } from 'react';
import { getCurrentUser } from '../services/api';

export function useAuthBootstrap() {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let mounted = true;
    const token = localStorage.getItem('token');

    if (!token) {
      setReady(true);
      return () => {
        mounted = false;
      };
    }

    getCurrentUser()
      .then((response) => {
        if (!mounted) return;
        const payload = response?.data?.response || response?.data;
        if (payload?.id) {
          localStorage.setItem('userId', String(payload.id));
        }
      })
      .catch(() => {
        if (!mounted) return;
        localStorage.removeItem('token');
        localStorage.removeItem('userId');
      })
      .finally(() => {
        if (mounted) {
          setReady(true);
        }
      });

    return () => {
      mounted = false;
    };
  }, []);

  return ready;
}
