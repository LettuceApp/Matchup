import { useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { getUser } from '../services/api';

const UserProfileRedirect = () => {
  const { userId } = useParams();
  const navigate = useNavigate();

  useEffect(() => {
    let mounted = true;
    const redirect = async () => {
      try {
        const res = await getUser(userId);
        if (!mounted) return;
        const payload = res.data?.response || res.data;
        const username = payload?.username;
        if (username) {
          navigate(`/users/${username}`, { replace: true });
          return;
        }
      } catch (err) {
        // fall through to fallback
      }
      if (mounted) {
        navigate(`/users/${userId}`, { replace: true });
      }
    };
    redirect();
    return () => {
      mounted = false;
    };
  }, [navigate, userId]);

  return null;
};

export default UserProfileRedirect;
