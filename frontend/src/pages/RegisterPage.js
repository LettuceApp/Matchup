import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { createUser, login } from '../services/api';

const RegisterPage = () => {
  const [email, setEmail]       = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError]       = useState(null);
  const navigate                = useNavigate();

  // If already authenticated, go home
  useEffect(() => {
    if (localStorage.getItem('token')) {
      navigate('/');
    }
  }, [navigate]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setError(null);

    try {
      // 1) Register the user
      await createUser({ email, username, password });
      // (createUser returns only the 201 + user payload :contentReference[oaicite:0]{index=0}:contentReference[oaicite:1]{index=1})

      // 2) Immediately log them in
      const res = await login({ email, password });
      // login returns { response: { token, id, email, username, avatar_path } } :contentReference[oaicite:2]{index=2}:contentReference[oaicite:3]{index=3}
      const { token, id } = res.data.response;

      // 3) Persist auth
      localStorage.setItem('token', token);
      localStorage.setItem('userId', id);

      // 4) Redirect
      navigate('/');
    } catch (err) {
      console.error('Registration/login error:', err.response?.data || err.message);
      setError(
        err.response?.data?.error ||
        err.response?.data?.message ||
        'Registration failed. Please try again.'
      );
    }
  };

  return (
    <div style={{ maxWidth: 400, margin: '2rem auto' }}>
      <h1>Register</h1>
      {error && <p style={{ color: 'red' }}>{error}</p>}

      <form onSubmit={handleSubmit}>
        <div style={{ marginBottom: '1rem' }}>
          <input
            type="email"
            placeholder="Email"
            value={email}
            onChange={e => setEmail(e.target.value)}
            required
            style={{ width: '100%' }}
          />
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <input
            placeholder="Username"
            value={username}
            onChange={e => setUsername(e.target.value)}
            required
            style={{ width: '100%' }}
          />
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={e => setPassword(e.target.value)}
            required
            style={{ width: '100%' }}
          />
        </div>

        <button type="submit" style={{ width: '100%' }}>
          Register &amp; Login
        </button>
      </form>

      <p style={{ marginTop: '1rem', textAlign: 'center' }}>
        Already have an account?{' '}
        <button onClick={() => navigate('/login')}>Login</button>
      </p>
    </div>
  );
};

export default RegisterPage;
