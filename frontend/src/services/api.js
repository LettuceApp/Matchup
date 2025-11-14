import axios from 'axios';

export const API = axios.create({
  // Make sure Netlify defines REACT_APP_API_BASE to: https://<your-api>.herokuapp.com/api/v1
  baseURL: process.env.REACT_APP_API_BASE,
});

// Reattach token on boot BEFORE any requests
const bootToken = localStorage.getItem('token');
if (bootToken) {
  API.defaults.headers.common.Authorization = `Bearer ${bootToken}`;
}

// Only clear token on 401 (don’t nuke it for network/CORS errors)
API.interceptors.response.use(
  (res) => res,
  (err) => {
    const status = err?.response?.status;
    if (status === 401) {
      localStorage.removeItem('token');
      delete API.defaults.headers.common.Authorization;
    }
    return Promise.reject(err);
  }
);

/* =========================
   Auth
   ========================= */

// Persist token on successful login + set header
export const login = async (data) => {
  const res = await API.post('/login', data, {
    headers: { 'Content-Type': 'application/json' },
  });
  const token = res?.data?.token; // adjust to your API response shape
  if (token) {
    localStorage.setItem('token', token);
    API.defaults.headers.common.Authorization = `Bearer ${token}`;
  }
  return res;
};

export const forgotPassword = (data) =>
  API.post('/password/forgot', data, { headers: { 'Content-Type': 'application/json' } });

export const resetPassword = (data) =>
  API.post('/password/reset', data, { headers: { 'Content-Type': 'application/json' } });

export const getCurrentUser = () => API.get('/me');

// Optional helper you can call on manual logout
export const logout = () => {
  localStorage.removeItem('token');
  localStorage.removeItem('userId'); // if you store it
  delete API.defaults.headers.common.Authorization;
};

/* =========================
   Users
   ========================= */

export const createUser = (data) =>
  API.post('/users', data, { headers: { 'Content-Type': 'application/json' } });

export const getUsers = () => API.get('/users');
export const getUser = (id) => API.get(`/users/${id}`);
export const updateUser = (id, data) =>
  API.put(`/users/${id}`, data, { headers: { 'Content-Type': 'application/json' } });

export const updateUserAvatar = (userId, formData) =>
  API.put(`/users/${userId}/avatar`, formData);

export const deleteUser = (id) => API.delete(`/users/${id}`);
export const getUserMatchups = (userId) => API.get(`/users/${userId}/matchups`);
export const getUserMatchup = (userId, matchupId) =>
  API.get(`/users/${userId}/matchups/${matchupId}`);

/* =========================
   Matchups
   ========================= */

// NOTE: add leading slash here
export const createMatchup = (userId, data) =>
  API.post(`/users/${userId}/create-matchup`, data, { headers: { 'Content-Type': 'application/json' } });

export const getMatchups = () => API.get('/matchups');
export const getMatchup = (id) => API.get(`/matchup/${id}`);
export const updateMatchup = (id, data) =>
  API.put(`/matchup/${id}`, data, { headers: { 'Content-Type': 'application/json' } });

export const deleteMatchup = (id) => API.delete(`/matchup/${id}`);

/* =========================
   Matchup Items
   ========================= */

export const incrementMatchupItemVotes = (id) => API.patch(`/matchup_items/${id}/vote`);
export const deleteMatchupItem = (id) => API.delete(`/matchup_items/${id}`);
export const updateMatchupItem = (itemId, updatedData) =>
  API.put(`/matchup_items/${itemId}`, updatedData, { headers: { 'Content-Type': 'application/json' } });

export const addItemToMatchup = (matchupId, itemData) =>
  API.post(`/matchups/${matchupId}/items`, itemData, { headers: { 'Content-Type': 'application/json' } });

/* =========================
   Likes
   ========================= */

export const getMatchupLikes = (matchupId) => API.get(`/likes/matchups/${matchupId}`);
export const likeMatchup = (id) => API.post(`/likes/matchups/${id}`);
export const unlikeMatchup = (id) => API.delete(`/likes/matchups/${id}`);
export const getUserLikes = (userId) => API.get(`/users/${userId}/likes`);

export default API;
