// frontend/src/services/api.js
import axios from "axios";

// 1) Prefer an explicit env var if it's set (for Netlify / production, etc.)
const envBase = (process.env.REACT_APP_API_BASE || "").trim();

// 2) Fallback for local dev + Docker: API is exposed on localhost:8888
const DEFAULT_API_BASE = "http://localhost:8888/api/v1";

export const API_BASE_URL = envBase || DEFAULT_API_BASE;

console.log("Using API base URL:", API_BASE_URL);

const API = axios.create({
  baseURL: API_BASE_URL,
  withCredentials: true,
});

// Attach auth token
API.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// -----------------------------------------
// AUTH
// -----------------------------------------
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

// USERS
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

// MATCHUPS
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

// MATCHUP ITEMS
export const incrementMatchupItemVotes = (id) =>
  API.patch(`/matchup_items/${id}/vote`);

export const deleteMatchupItem = (id) => API.delete(`/matchup_items/${id}`);
export const updateMatchupItem = (itemId, updatedData) =>
  API.put(`/matchup_items/${itemId}`, updatedData);

export const addItemToMatchup = (matchupId, itemData) =>
  API.post(`/matchups/${matchupId}/items`, itemData);

// LIKES
export const getMatchupLikes = (matchupId) =>
  API.get(`/matchups/${matchupId}/likes`);

export const likeMatchup = (matchupId) =>
  API.post(`/matchups/${matchupId}/likes`);

export const unlikeMatchup = (matchupId) =>
  API.delete(`/matchups/${matchupId}/likes`);

export const getUserLikes = (userId) => API.get(`/users/${userId}/likes`);

// COMMENTS
export const createComment = (matchupId, commentData) =>
  API.post(`/matchups/${matchupId}/comments`, commentData);

export const getComments = (matchupId) =>
  API.get(`/matchups/${matchupId}/comments`);

export const updateComment = (id, commentData) =>
  API.put(`/comments/${id}`, commentData);

export const deleteComment = (id) => API.delete(`/comments/${id}`);

// CURRENT USER
export const getCurrentUser = () => API.get('/me');

// LEADERBOARD
export const getPopularMatchups = () => API.get('/matchups/popular');

// ADMIN
export const adminGetUsers = (params = {}) => API.get('/admin/users', { params });
export const adminUpdateUserRole = (userId, data) => API.patch(`/admin/users/${userId}/role`, data);
export const adminGetMatchups = (params = {}) => API.get('/admin/matchups', { params });
export const adminDeleteMatchup = (matchupId) => API.delete(`/admin/matchups/${matchupId}`);

export default API;
