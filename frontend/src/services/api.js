
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
  const token = res.data.token; // adjust to your API shape
  if (token) {
    localStorage.setItem('token', token);
    API.defaults.headers.Authorization = `Bearer ${token}`;
  }
  return res;
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
  formData.append('file', file); // MUST be 'file' to match c.FormFile("file")

  return API.put(`/users/${userId}/avatar`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  });
};

export const deleteUser = (id) => API.delete(`/users/${id}`);
export const getUserMatchups = (userId) => API.get(`/users/${userId}/matchups`);
export const getUserMatchup = (userId, matchupId) => API.get(`/users/${userId}/matchups/${matchupId}`);

// Matchup Management
export const createMatchup = (userId, data) => API.post(`users/${userId}/create-matchup`, data);
export const getMatchups = () => API.get('/matchups');
export const getMatchup = (id) => API.get(`/matchup/${id}`);
export const updateMatchup = (id, data) => API.put(`/matchup/${id}`, data);
export const deleteMatchup = (id) => API.delete(`/matchup/${id}`);

// Matchup Items
export const incrementMatchupItemVotes = (id) => API.patch(`/matchup_items/${id}/vote`);
export const deleteMatchupItem = (id) => API.delete(`/matchup_items/${id}`);
export const updateMatchupItem = (itemId, updatedData) =>  API.put(`/matchup_items/${itemId}`, updatedData);
export const addItemToMatchup = (matchupId, itemData) => API.post(`/matchups/${matchupId}/items`, itemData);

// Likes Management
export const getMatchupLikes = (matchupId) => API.get(`/likes/matchups/${matchupId}`);
export const likeMatchup = (id) => API.post(`/likes/matchups/${id}`);
export const unlikeMatchup = (id) => API.delete(`/likes/matchups/${id}`);
export const getUserLikes = (userId) => API.get(`/users/${userId}/likes`);

// Comments Management
export const createComment = (matchupId, commentData) => API.post(`/matchups/${matchupId}/comments`, commentData);
export const getComments = (matchupId) => API.get(`/matchups/${matchupId}/comments`);
export const updateComment = (id, commentData) => API.put(`/comments/${id}`, commentData);
export const deleteComment = (id) => API.delete(`/comments/${id}`);

// Auth helpers
export const getCurrentUser = () => API.get('/me');

export default API;
