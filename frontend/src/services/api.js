import axios from 'axios';

// Create an Axios instance with a base URL
const API = axios.create({
  baseURL: process.env.REACT_APP_API_BASE,
});

// Attach the Authorization token to requests if available
API.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// User Auth
export const login = async (data) => {
  const res = await API.post('/login', data);
  const payload = res?.data?.response || res?.data || {};

  if (payload.token) {
    localStorage.setItem('token', payload.token);
    API.defaults.headers.Authorization = `Bearer ${payload.token}`;
  }

  if (payload.id) {
    localStorage.setItem('userId', String(payload.id));
  }

  if (typeof payload.is_admin === 'boolean') {
    localStorage.setItem('isAdmin', payload.is_admin ? 'true' : 'false');
  } else {
    localStorage.removeItem('isAdmin');
  }

  return payload;
};

export const forgotPassword = (data) => API.post('/password/forgot', data);
export const resetPassword = (data) => API.post('/password/reset', data);

// User Management
export const createUser = (data) => API.post('/users', data);
export const getUsers = () => API.get('/users');
export const getUser = (id) => API.get(`/users/${id}`);
export const updateUser = (id, data) => API.put(`/users/${id}`, data);
export const updateUserAvatar = (userId, file) => {
  const formData = new FormData();
  formData.append('file', file);

  return API.put(`/users/${userId}/avatar`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
};

export const deleteUser = (id) => API.delete(`/users/${id}`);

// Matchup Management (paginated)
export const getMatchups = (page = 1, limit = 10) =>
  API.get('/matchups', { params: { page, limit } });

export const getUserMatchups = (userId, page = 1, limit = 10) =>
  API.get(`/users/${userId}/matchups`, { params: { page, limit } });

export const getUserMatchup = (userId, matchupId) =>
  API.get(`/users/${userId}/matchups/${matchupId}`);

export const createMatchup = (userId, data) =>
  API.post(`/users/${userId}/matchups`, data);

export const getMatchup = (id) => API.get(`/matchups/${id}`);
export const updateMatchup = (id, data) => API.put(`/matchups/${id}`, data);
export const deleteMatchup = (id) => API.delete(`/matchups/${id}`);

// Matchup Items
export const incrementMatchupItemVotes = (id) =>
  API.patch(`/matchup_items/${id}/vote`);

export const deleteMatchupItem = (id) => API.delete(`/matchup_items/${id}`);

export const updateMatchupItem = (itemId, updatedData) =>
  API.put(`/matchup_items/${itemId}`, updatedData);

export const addItemToMatchup = (matchupId, itemData) =>
  API.post(`/matchups/${matchupId}/items`, itemData);

// Likes Management
export const getMatchupLikes = (matchupId) =>
  API.get(`/matchups/${matchupId}/likes`);

export const likeMatchup = (matchupId) =>
  API.post(`/matchups/${matchupId}/likes`);

export const unlikeMatchup = (matchupId) =>
  API.delete(`/matchups/${matchupId}/likes`);

export const getUserLikes = (userId) => API.get(`/users/${userId}/likes`);

// Comments Management
export const createComment = (matchupId, commentData) =>
  API.post(`/matchups/${matchupId}/comments`, commentData);

export const getComments = (matchupId) =>
  API.get(`/matchups/${matchupId}/comments`);

export const updateComment = (id, commentData) =>
  API.put(`/comments/${id}`, commentData);

export const deleteComment = (id) => API.delete(`/comments/${id}`);

// Auth helpers
export const getCurrentUser = () => API.get('/me');

// Leaderboard
export const getPopularMatchups = () => API.get('/matchups/popular');

// Admin APIs
export const adminGetUsers = (params = {}) => API.get('/admin/users', { params });
export const adminUpdateUserRole = (userId, data) => API.patch(`/admin/users/${userId}/role`, data);
export const adminGetMatchups = (params = {}) => API.get('/admin/matchups', { params });
export const adminDeleteMatchup = (matchupId) => API.delete(`/admin/matchups/${matchupId}`);

export default API;
