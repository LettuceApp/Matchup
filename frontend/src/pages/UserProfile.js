import React, { useState, useEffect } from 'react';
import { useParams, Link }        from 'react-router-dom';
import API, { getUserMatchups, getUser } from '../services/api';
import NavigationBar              from '../components/NavigationBar';
import ProfilePic                 from '../components/ProfilePic'; // ← use this

const UserProfile = () => {
  const { userId } = useParams();
  const [matchups, setMatchups] = useState([]);
  const [user, setUser]         = useState(null);
  const [loading, setLoading]   = useState(true);
  const [error, setError]       = useState(null);

  useEffect(() => {
    (async () => {
      try {
        const [uRes, mRes] = await Promise.all([
          getUser(userId),
          getUserMatchups(userId)
        ]);
        setUser(uRes.data.response || uRes.data);
        setMatchups(mRes.data.response || mRes.data);
      } catch (err) {
        console.error(err);
        setError('Failed to load user data.');
      } finally {
        setLoading(false);
      }
    })();
  }, [userId]);

  if (loading) return <p>Loading…</p>;
  if (error)   return <p style={{ color: 'red' }}>{error}</p>;

  return (
    <div>
      <NavigationBar />

      <div style={{ position: 'relative', padding: '1rem 1rem 2rem' }}>
        <h1>User Profile</h1>

        {/* replace your old avatar code with this: */}
        {user && (
          <div
            style={{
              position: 'absolute',
              top: '1rem',
              right: '1rem'
            }}
          >
            <ProfilePic
              userId={userId}
              editable
              size={80}
            />
          </div>
        )}
      </div>

      {user && (
        <div style={{ padding: '1rem' }}>
          <h2>{user.username || user.name}</h2>
          <p>{user.email}</p>
        </div>
      )}

      <h2 style={{ paddingLeft: '1rem' }}>Matchups by User {userId}</h2>
      {matchups.length > 0 ? (
        matchups.map(m => (
          <div
            key={m.id}
            style={{ borderBottom: '1px solid #ccc', padding: '1rem' }}
          >
            <h3>
              <Link to={`/users/${userId}/matchup/${m.id}`}>
                {m.title}
              </Link>
            </h3>
            <p>{m.content || m.description}</p>
          </div>
        ))
      ) : (
        <p style={{ paddingLeft: '1rem' }}>No matchups available.</p>
      )}
    </div>
  );
};

export default UserProfile;
