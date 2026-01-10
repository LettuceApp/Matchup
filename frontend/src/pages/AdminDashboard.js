import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import NavigationBar from '../components/NavigationBar';
import {
  adminGetUsers,
  adminUpdateUserRole,
  adminDeleteUser,
  adminGetMatchups,
  adminDeleteMatchup,
  adminGetBrackets,
  adminDeleteBracket,
} from '../services/api';
import '../styles/AdminDashboard.css';

const AdminDashboard = () => {
  const [users, setUsers] = useState([]);
  const [matchups, setMatchups] = useState([]);
  const [brackets, setBrackets] = useState([]);
  const [userPage, setUserPage] = useState(1);
  const [matchupPage, setMatchupPage] = useState(1);
  const [bracketPage, setBracketPage] = useState(1);
  const [userPagination, setUserPagination] = useState(null);
  const [matchupPagination, setMatchupPagination] = useState(null);
  const [bracketPagination, setBracketPagination] = useState(null);
  const [userSearchInput, setUserSearchInput] = useState('');
  const [userSearchQuery, setUserSearchQuery] = useState('');
  const [matchupSearchInput, setMatchupSearchInput] = useState('');
  const [matchupSearchQuery, setMatchupSearchQuery] = useState('');
  const [bracketSearchInput, setBracketSearchInput] = useState('');
  const [bracketSearchQuery, setBracketSearchQuery] = useState('');
  const [loadingUsers, setLoadingUsers] = useState(false);
  const [loadingMatchups, setLoadingMatchups] = useState(false);
  const [loadingBrackets, setLoadingBrackets] = useState(false);
  const [userError, setUserError] = useState(null);
  const [matchupError, setMatchupError] = useState(null);
  const [bracketError, setBracketError] = useState(null);
  const [actionMessage, setActionMessage] = useState('');

  const navigate = useNavigate();

  const handleUnauthorized = useCallback(() => {
    localStorage.removeItem('token');
    localStorage.removeItem('userId');
    localStorage.removeItem('isAdmin');
    navigate('/login', { replace: true });
  }, [navigate]);

  const handleAdminAccessLost = useCallback(() => {
    localStorage.setItem('isAdmin', 'false');
    navigate('/home', { replace: true });
  }, [navigate]);

  const fetchUsers = useCallback(() => {
    setLoadingUsers(true);
    setUserError(null);
    const params = { page: userPage, limit: 10 };
    if (userSearchQuery) params.search = userSearchQuery;

    adminGetUsers(params)
      .then((res) => {
        const payload = res?.data?.response || res?.data;
        setUsers(payload?.users || []);
        setUserPagination(payload?.pagination || null);
      })
      .catch((err) => {
        const status = err?.response?.status;
        if (status === 401) {
          handleUnauthorized();
          return;
        }
        if (status === 403) {
          handleAdminAccessLost();
          return;
        }
        setUserError('Unable to load users right now.');
      })
      .finally(() => setLoadingUsers(false));
  }, [userPage, userSearchQuery, handleUnauthorized, handleAdminAccessLost]);

  const fetchMatchups = useCallback(() => {
    setLoadingMatchups(true);
    setMatchupError(null);
    const params = { page: matchupPage, limit: 10 };
    if (matchupSearchQuery) params.search = matchupSearchQuery;

    adminGetMatchups(params)
      .then((res) => {
        const payload = res?.data?.response || res?.data;
        setMatchups(payload?.matchups || []);
        setMatchupPagination(payload?.pagination || null);
      })
      .catch((err) => {
        const status = err?.response?.status;
        if (status === 401) {
          handleUnauthorized();
          return;
        }
        if (status === 403) {
          handleAdminAccessLost();
          return;
        }
        setMatchupError('Unable to load matchups right now.');
      })
      .finally(() => setLoadingMatchups(false));
  }, [matchupPage, matchupSearchQuery, handleUnauthorized, handleAdminAccessLost]);

  const fetchBrackets = useCallback(() => {
    setLoadingBrackets(true);
    setBracketError(null);
    const params = { page: bracketPage, limit: 10 };
    if (bracketSearchQuery) params.search = bracketSearchQuery;

    adminGetBrackets(params)
      .then((res) => {
        const payload = res?.data?.response || res?.data;
        setBrackets(payload?.brackets || []);
        setBracketPagination(payload?.pagination || null);
      })
      .catch((err) => {
        const status = err?.response?.status;
        if (status === 401) {
          handleUnauthorized();
          return;
        }
        if (status === 403) {
          handleAdminAccessLost();
          return;
        }
        setBracketError('Unable to load brackets right now.');
      })
      .finally(() => setLoadingBrackets(false));
  }, [bracketPage, bracketSearchQuery, handleUnauthorized, handleAdminAccessLost]);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  useEffect(() => {
    fetchMatchups();
  }, [fetchMatchups]);

  useEffect(() => {
    fetchBrackets();
  }, [fetchBrackets]);

  const handleUserSearchSubmit = (e) => {
    e.preventDefault();
    setUserPage(1);
    setUserSearchQuery(userSearchInput.trim());
  };

  const handleMatchupSearchSubmit = (e) => {
    e.preventDefault();
    setMatchupPage(1);
    setMatchupSearchQuery(matchupSearchInput.trim());
  };

  const handleBracketSearchSubmit = (e) => {
    e.preventDefault();
    setBracketPage(1);
    setBracketSearchQuery(bracketSearchInput.trim());
  };

  const handleToggleAdmin = async (user) => {
    const nextValue = !user.is_admin;
    setActionMessage('');

    try {
      await adminUpdateUserRole(user.id, { is_admin: nextValue });
      setActionMessage(`${user.username} is now ${nextValue ? 'an admin' : 'a user'}.`);
      const currentUserId = localStorage.getItem('userId');
      if (currentUserId && Number(currentUserId) === user.id) {
        localStorage.setItem('isAdmin', nextValue ? 'true' : 'false');
        if (!nextValue) {
          handleAdminAccessLost();
          return;
        }
      }
      fetchUsers();
    } catch (err) {
      const status = err?.response?.status;
      if (status === 401) {
        handleUnauthorized();
        return;
      }
      if (status === 403) {
        handleAdminAccessLost();
        return;
      }
      setActionMessage('Unable to update user role.');
    }
  };

  const handleDeleteUser = async (user) => {
    setActionMessage('');
    if (!window.confirm(`Delete ${user.username}? This cannot be undone.`)) {
      return;
    }

    try {
      await adminDeleteUser(user.id);
      setActionMessage('User deleted.');
      fetchUsers();
      fetchMatchups();
      fetchBrackets();
    } catch (err) {
      const status = err?.response?.status;
      if (status === 401) {
        handleUnauthorized();
        return;
      }
      if (status === 403) {
        handleAdminAccessLost();
        return;
      }
      setActionMessage('Unable to delete user.');
    }
  };

  const handleDeleteMatchup = async (matchupId) => {
    setActionMessage('');
    if (!window.confirm('Delete this matchup? This cannot be undone.')) {
      return;
    }
    try {
      await adminDeleteMatchup(matchupId);
      setActionMessage('Matchup deleted.');
      fetchMatchups();
    } catch (err) {
      const status = err?.response?.status;
      if (status === 401) {
        handleUnauthorized();
        return;
      }
      if (status === 403) {
        handleAdminAccessLost();
        return;
      }
      setActionMessage('Unable to delete matchup.');
    }
  };

  const handleDeleteBracket = async (bracketId) => {
    setActionMessage('');
    if (!window.confirm('Delete this bracket and all associated matchups?')) {
      return;
    }

    try {
      await adminDeleteBracket(bracketId);
      setActionMessage('Bracket deleted.');
      fetchBrackets();
      fetchMatchups();
    } catch (err) {
      const status = err?.response?.status;
      if (status === 401) {
        handleUnauthorized();
        return;
      }
      if (status === 403) {
        handleAdminAccessLost();
        return;
      }
      setActionMessage('Unable to delete bracket.');
    }
  };

  const renderPagination = (pagination, page, setPage) => {
    if (!pagination || pagination.total_pages <= 1) {
      return null;
    }
    return (
      <div className="admin-pagination">
        <button type="button" disabled={page <= 1} onClick={() => setPage((prev) => Math.max(1, prev - 1))}>
          Previous
        </button>
        <span>
          Page {pagination.page} of {pagination.total_pages}
        </span>
        <button
          type="button"
          disabled={page >= pagination.total_pages}
          onClick={() => setPage((prev) => prev + 1)}
        >
          Next
        </button>
      </div>
    );
  };

  return (
    <div className="admin-dashboard">
      <NavigationBar />
      <main className="admin-main">
        <header className="admin-header">
          <div>
            <p className="admin-overline">Admin Console</p>
            <h1>Moderate Matchup Hub</h1>
            <p>Review users and remove content that violates your guidelines.</p>
          </div>
          {actionMessage && <div className="admin-toast">{actionMessage}</div>}
        </header>

        <section className="admin-section">
          <div className="admin-section-header">
            <div>
              <h2>Users</h2>
              <p>Promote or demote admins and keep tabs on new accounts.</p>
            </div>
            <form className="admin-search" onSubmit={handleUserSearchSubmit}>
              <input
                type="text"
                placeholder="Search by email or username"
                value={userSearchInput}
                onChange={(e) => setUserSearchInput(e.target.value)}
              />
              <button type="submit">Search</button>
            </form>
          </div>

          {loadingUsers && <div className="admin-status-card">Loading users…</div>}
          {userError && !loadingUsers && <div className="admin-status-card admin-status-card--error">{userError}</div>}

          {!loadingUsers && !userError && (
            <>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Username</th>
                    <th>Email</th>
                    <th>Role</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.length === 0 && (
                    <tr>
                      <td colSpan="6" className="admin-empty">
                        No users found.
                      </td>
                    </tr>
                  )}
                  {users.map((user) => (
                    <tr key={user.id}>
                      <td>{user.id}</td>
                      <td>{user.username}</td>
                      <td>{user.email}</td>
                      <td>{user.is_admin ? 'Admin' : 'User'}</td>
                      <td>{new Date(user.created_at).toLocaleDateString()}</td>
                      <td>
                        <div className="admin-action-group">
                          <button
                            type="button"
                            className="admin-link"
                            onClick={() => handleToggleAdmin(user)}
                          >
                            {user.is_admin ? 'Remove Admin' : 'Make Admin'}
                          </button>
                          <button
                            type="button"
                            className="admin-link admin-link--danger"
                            onClick={() => handleDeleteUser(user)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {renderPagination(userPagination, userPage, setUserPage)}
            </>
          )}
        </section>

        <section className="admin-section">
          <div className="admin-section-header">
            <div>
              <h2>Matchups</h2>
              <p>Review and remove problematic matchups.</p>
            </div>
            <form className="admin-search" onSubmit={handleMatchupSearchSubmit}>
              <input
                type="text"
                placeholder="Search by title or content"
                value={matchupSearchInput}
                onChange={(e) => setMatchupSearchInput(e.target.value)}
              />
              <button type="submit">Search</button>
            </form>
          </div>

          {loadingMatchups && <div className="admin-status-card">Loading matchups…</div>}
          {matchupError && !loadingMatchups && (
            <div className="admin-status-card admin-status-card--error">{matchupError}</div>
          )}

          {!loadingMatchups && !matchupError && (
            <>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Title</th>
                    <th>Author</th>
                    <th>Items</th>
                    <th>Likes</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {matchups.length === 0 && (
                    <tr>
                      <td colSpan="7" className="admin-empty">
                        No matchups found.
                      </td>
                    </tr>
                  )}
                  {matchups.map((matchup) => (
                    <tr key={matchup.id}>
                      <td>{matchup.id}</td>
                      <td>{matchup.title}</td>
                      <td>
                        {matchup.author_username} (#{matchup.author_id})
                      </td>
                      <td>
                        {matchup.items.map((item) => item.item).join(', ') || '—'}
                      </td>
                      <td>{matchup.likes_count}</td>
                      <td>{new Date(matchup.created_at).toLocaleDateString()}</td>
                      <td>
                        <button
                          type="button"
                          className="admin-link admin-link--danger"
                          onClick={() => handleDeleteMatchup(matchup.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {renderPagination(matchupPagination, matchupPage, setMatchupPage)}
            </>
          )}
        </section>

        <section className="admin-section">
          <div className="admin-section-header">
            <div>
              <h2>Brackets</h2>
              <p>Review and remove brackets along with their matchups.</p>
            </div>
            <form className="admin-search" onSubmit={handleBracketSearchSubmit}>
              <input
                type="text"
                placeholder="Search by title"
                value={bracketSearchInput}
                onChange={(e) => setBracketSearchInput(e.target.value)}
              />
              <button type="submit">Search</button>
            </form>
          </div>

          {loadingBrackets && <div className="admin-status-card">Loading brackets…</div>}
          {bracketError && !loadingBrackets && (
            <div className="admin-status-card admin-status-card--error">{bracketError}</div>
          )}

          {!loadingBrackets && !bracketError && (
            <>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Title</th>
                    <th>Author</th>
                    <th>Status</th>
                    <th>Round</th>
                    <th>Likes</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {brackets.length === 0 && (
                    <tr>
                      <td colSpan="8" className="admin-empty">
                        No brackets found.
                      </td>
                    </tr>
                  )}
                  {brackets.map((bracket) => (
                    <tr key={bracket.id}>
                      <td>{bracket.id}</td>
                      <td>{bracket.title}</td>
                      <td>
                        {bracket.author_username} (#{bracket.author_id})
                      </td>
                      <td>{bracket.status}</td>
                      <td>{bracket.current_round}</td>
                      <td>{bracket.likes_count}</td>
                      <td>{new Date(bracket.created_at).toLocaleDateString()}</td>
                      <td>
                        <button
                          type="button"
                          className="admin-link admin-link--danger"
                          onClick={() => handleDeleteBracket(bracket.id)}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {renderPagination(bracketPagination, bracketPage, setBracketPage)}
            </>
          )}
        </section>
      </main>
    </div>
  );
};

export default AdminDashboard;
