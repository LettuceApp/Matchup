// frontend/src/services/api.js
import axios from "axios";


let API_BASE_URL = process.env.REACT_APP_API_BASE;

if (!API_BASE_URL || API_BASE_URL.includes("localhost")) {
  if (window.location.hostname.includes("onrender.com")) {
    API_BASE_URL = "https://matchup-vhl6.onrender.com/api/v1";
  } else {
    API_BASE_URL = "http://localhost:8888/api/v1";
  }
}

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

  if (payload.username) {
    localStorage.setItem('username', String(payload.username));
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
export const updateUserPrivacy = (id, isPrivate) =>
  API.patch(`/users/${id}/privacy`, { is_private: isPrivate });
export const followUser = (id) => API.post(`/users/${id}/follow`);
export const unfollowUser = (id) => API.delete(`/users/${id}/follow`);
export const getUserRelationship = (id) => API.get(`/users/${id}/relationship`);
export const getUserFollowers = (id, params = {}) =>
  API.get(`/users/${id}/followers`, { params });
export const getUserFollowing = (id, params = {}) =>
  API.get(`/users/${id}/following`, { params });

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
export const getUserMatchupVotes = (userId) => API.get(`/users/${userId}/matchup_votes`);

// BRACKET LIKES
export const getBracketLikes = (bracketId) =>
  API.get(`/brackets/${bracketId}/likes`);

export const likeBracket = (bracketId) =>
  API.post(`/brackets/${bracketId}/likes`);

export const unlikeBracket = (bracketId) =>
  API.delete(`/brackets/${bracketId}/likes`);

export const getUserBracketLikes = (userId) =>
  API.get(`/users/${userId}/bracket_likes`);

// COMMENTS
export const createComment = (matchupId, commentData) =>
  API.post(`/matchups/${matchupId}/comments`, commentData);

export const getComments = (matchupId) =>
  API.get(`/matchups/${matchupId}/comments`);

export const getBracketComments = (bracketId) =>
  API.get(`/brackets/${bracketId}/comments`);

export const overrideMatchupWinner = (matchupId, winnerItemId) =>
  API.post(`/matchups/${matchupId}/override-winner`, {
    winner_item_id: winnerItemId,
  });

export const createBracketComment = (bracketId, commentData) =>
  API.post(`/brackets/${bracketId}/comments`, commentData);

export const updateComment = (id, commentData) =>
  API.put(`/comments/${id}`, commentData);

export const deleteComment = (id) => API.delete(`/comments/${id}`);

export const deleteBracketComment = (id) => API.delete(`/bracket_comments/${id}`);

// CURRENT USER
export const getCurrentUser = () => API.get('/me');

// LEADERBOARD
export const getPopularMatchups = () => API.get('/matchups/popular');
export const getPopularBrackets = () => API.get('/brackets/popular');
export const getHomeSummary = (userId) => {
  const params = {};
  if (userId) {
    params.user_id = userId;
  }
  return API.get('/home', { params });
};

// BRACKET
export const createBracket = (userId, data) =>
  API.post(`/users/${userId}/brackets`, data);

export const getUserBrackets = (userId, params = {}) =>
  API.get(`/users/${userId}/brackets`, { params });

export const getBracket = (id) =>
  API.get(`/brackets/${id}`);

export const getBracketMatchups = (id) =>
  API.get(`/brackets/${id}/matchups`);

export const getBracketSummary = (id, viewerId) => {
  const params = {};
  if (viewerId) {
    params.viewer_id = viewerId;
  }
  return API.get(`/brackets/${id}/summary`, { params });
};

export const attachMatchupToBracket = (bracketId, data) =>
  API.post(`/brackets/${bracketId}/matchups`, data);

export const updateBracket = (id, data) =>
  API.put(`/brackets/${id}`, data);

export const advanceBracket = (id) =>
  API.post(`/brackets/${id}/advance`);

export const archiveBracket = (id) =>
  API.put(`/brackets/${id}`, { status: "archived" });

export const activateMatchup = (matchupId) =>
  API.post(`/matchups/${matchupId}/activate`);



export const deleteBracket = (id) =>
  API.delete(`/brackets/${id}`);

export const completeMatchup = (matchupId) =>
  API.post(`/matchups/${matchupId}/complete`);


export const resolveTieAndAdvance = (matchupId, winnerItemId) =>
  API.post(`/matchups/${matchupId}/resolve-and-advance`, {
    winner_item_id: winnerItemId,
  });



// ADMIN
export const adminGetUsers = (params = {}) => API.get('/admin/users', { params });
export const adminUpdateUserRole = (userId, data) => API.patch(`/admin/users/${userId}/role`, data);
export const adminDeleteUser = (userId) => API.delete(`/admin/users/${userId}`);
export const adminGetMatchups = (params = {}) => API.get('/admin/matchups', { params });
export const adminDeleteMatchup = (matchupId) => API.delete(`/admin/matchups/${matchupId}`);
export const adminGetBrackets = (params = {}) => API.get('/admin/brackets', { params });
export const adminDeleteBracket = (bracketId) => API.delete(`/admin/brackets/${bracketId}`);

export default API;
