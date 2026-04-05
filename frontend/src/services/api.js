// frontend/src/services/api.js
import axios from "axios";

let API_BASE_URL = process.env.REACT_APP_API_BASE;

if (!API_BASE_URL || API_BASE_URL.includes("localhost")) {
  if (window.location.hostname.includes("onrender.com")) {
    API_BASE_URL = "https://matchup-vhl6.onrender.com";
  } else if (window.location.hostname === "localhost") {
    API_BASE_URL = "http://localhost:8888";
  } else {
    // K8s / production: Ingress routes to the API service on the same host
    API_BASE_URL = "";
  }
}

console.log("Using API base URL:", API_BASE_URL);

const API = axios.create({
  baseURL: API_BASE_URL,
});

// Attach auth token and Connect protocol headers
API.interceptors.request.use((config) => {
  const token = localStorage.getItem('token');
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  config.headers['Connect-Protocol-Version'] = '1';
  return config;
});

// Helper: call a ConnectRPC method
const rpc = (service, method, body = {}) =>
  API.post(`/${service}/${method}`, body);

// -----------------------------------------
// AUTH
// -----------------------------------------
export const login = async (data) => {
  const res = await rpc('auth.v1.AuthService', 'Login', {
    email: data.email,
    password: data.password,
  });
  const payload = res?.data || {};

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

export const forgotPassword = (data) =>
  rpc('auth.v1.AuthService', 'ForgotPassword', { email: data.email });

export const resetPassword = (data) =>
  rpc('auth.v1.AuthService', 'ResetPassword', data);

// -----------------------------------------
// USERS
// -----------------------------------------
export const createUser = (data) =>
  rpc('user.v1.UserService', 'CreateUser', data);

export const getUsers = () =>
  rpc('user.v1.UserService', 'ListUsers', {});

export const getUser = (id) =>
  rpc('user.v1.UserService', 'GetUser', { id });

export const getCurrentUser = () =>
  rpc('user.v1.UserService', 'GetCurrentUser', {});

export const updateUser = (id, data) =>
  rpc('user.v1.UserService', 'UpdateUser', { id, ...data });

export const updateUserPrivacy = (id, isPrivate) =>
  rpc('user.v1.UserService', 'UpdateUserPrivacy', { id, is_private: isPrivate });

export const updateUserAvatar = (userId, file) => {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = async () => {
      try {
        // Strip the data URL prefix to get raw base64
        const base64 = reader.result.split(',')[1];
        const res = await rpc('user.v1.UserService', 'UpdateAvatar', {
          id: userId,
          avatar_data: base64,
          content_type: file.type,
        });
        resolve(res);
      } catch (err) {
        reject(err);
      }
    };
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
};

export const deleteUser = (id) =>
  rpc('user.v1.UserService', 'DeleteUser', { id });

export const followUser = (id) =>
  rpc('user.v1.UserService', 'FollowUser', { id });

export const unfollowUser = (id) =>
  rpc('user.v1.UserService', 'UnfollowUser', { id });

export const getUserRelationship = (id) =>
  rpc('user.v1.UserService', 'GetRelationship', { id });

export const getUserFollowers = (id, params = {}) =>
  rpc('user.v1.UserService', 'GetFollowers', { id, ...params });

export const getUserFollowing = (id, params = {}) =>
  rpc('user.v1.UserService', 'GetFollowing', { id, ...params });

// -----------------------------------------
// MATCHUPS
// -----------------------------------------
export const getMatchups = (page = 1, limit = 10) =>
  rpc('matchup.v1.MatchupService', 'ListMatchups', { page, limit });

export const getUserMatchups = (userId, page = 1, limit = 10) =>
  rpc('matchup.v1.MatchupService', 'GetUserMatchups', { user_id: userId, page, limit });

// getUserMatchup kept for back-compat — fetches a single matchup by its id
export const getUserMatchup = (_userId, matchupId) =>
  rpc('matchup.v1.MatchupService', 'GetMatchup', { id: matchupId });

export const getMatchup = (id) =>
  rpc('matchup.v1.MatchupService', 'GetMatchup', { id });

export const createMatchup = (userId, data) =>
  rpc('matchup.v1.MatchupService', 'CreateMatchup', { user_id: userId, ...data });

export const updateMatchup = (id, data) =>
  rpc('matchup.v1.MatchupService', 'UpdateMatchup', { id, ...data });

export const deleteMatchup = (id) =>
  rpc('matchup.v1.MatchupService', 'DeleteMatchup', { id });

export const getPopularMatchups = () =>
  rpc('matchup.v1.MatchupService', 'GetPopularMatchups', {});

export const overrideMatchupWinner = (matchupId, winnerItemId) =>
  rpc('matchup.v1.MatchupService', 'OverrideMatchupWinner', {
    id: matchupId,
    winner_item_id: winnerItemId,
  });

export const activateMatchup = (matchupId) =>
  rpc('matchup.v1.MatchupService', 'ActivateMatchup', { id: matchupId });

export const completeMatchup = (matchupId) =>
  rpc('matchup.v1.MatchupService', 'CompleteMatchup', { id: matchupId });

// -----------------------------------------
// MATCHUP ITEMS
// -----------------------------------------
export const incrementMatchupItemVotes = (id) =>
  rpc('matchup.v1.MatchupItemService', 'VoteItem', { id });

export const deleteMatchupItem = (id) =>
  rpc('matchup.v1.MatchupItemService', 'DeleteItem', { id });

export const updateMatchupItem = (itemId, updatedData) =>
  rpc('matchup.v1.MatchupItemService', 'UpdateItem', { id: itemId, ...updatedData });

export const addItemToMatchup = (matchupId, itemData) =>
  rpc('matchup.v1.MatchupItemService', 'AddItem', { matchup_id: matchupId, ...itemData });

export const getUserMatchupVotes = (userId) =>
  rpc('matchup.v1.MatchupItemService', 'GetUserVotes', { user_id: userId });

// -----------------------------------------
// LIKES
// -----------------------------------------
export const getMatchupLikes = (matchupId) =>
  rpc('like.v1.LikeService', 'GetLikes', { matchup_id: matchupId });

export const likeMatchup = (matchupId) =>
  rpc('like.v1.LikeService', 'LikeMatchup', { matchup_id: matchupId });

export const unlikeMatchup = (matchupId) =>
  rpc('like.v1.LikeService', 'UnlikeMatchup', { matchup_id: matchupId });

export const getUserLikes = (userId) =>
  rpc('like.v1.LikeService', 'GetUserLikes', { user_id: userId });

// -----------------------------------------
// BRACKET LIKES
// -----------------------------------------
export const getBracketLikes = (bracketId) =>
  rpc('like.v1.LikeService', 'GetBracketLikes', { bracket_id: bracketId });

export const likeBracket = (bracketId) =>
  rpc('like.v1.LikeService', 'LikeBracket', { bracket_id: bracketId });

export const unlikeBracket = (bracketId) =>
  rpc('like.v1.LikeService', 'UnlikeBracket', { bracket_id: bracketId });

export const getUserBracketLikes = (userId) =>
  rpc('like.v1.LikeService', 'GetUserBracketLikes', { user_id: userId });

// -----------------------------------------
// COMMENTS
// -----------------------------------------
export const getComments = (matchupId) =>
  rpc('comment.v1.CommentService', 'GetComments', { matchup_id: matchupId });

export const createComment = (matchupId, commentData) =>
  rpc('comment.v1.CommentService', 'CreateComment', { matchup_id: matchupId, ...commentData });

export const updateComment = (id, commentData) =>
  rpc('comment.v1.CommentService', 'UpdateComment', { id, ...commentData });

export const deleteComment = (id) =>
  rpc('comment.v1.CommentService', 'DeleteComment', { id });

export const getBracketComments = (bracketId) =>
  rpc('comment.v1.CommentService', 'GetBracketComments', { bracket_id: bracketId });

export const createBracketComment = (bracketId, commentData) =>
  rpc('comment.v1.CommentService', 'CreateBracketComment', { bracket_id: bracketId, ...commentData });

export const deleteBracketComment = (id) =>
  rpc('comment.v1.CommentService', 'DeleteBracketComment', { id });

// -----------------------------------------
// HOME
// -----------------------------------------
export const getHomeSummary = (userId) => {
  const body = {};
  if (userId) body.user_id = userId;
  return rpc('home.v1.HomeService', 'GetHomeSummary', body);
};

// -----------------------------------------
// BRACKETS
// -----------------------------------------
export const createBracket = (userId, data) =>
  rpc('bracket.v1.BracketService', 'CreateBracket', { user_id: userId, ...data });

export const getUserBrackets = (userId, params = {}) =>
  rpc('bracket.v1.BracketService', 'GetUserBrackets', { user_id: userId, ...params });

export const getBracket = (id) =>
  rpc('bracket.v1.BracketService', 'GetBracket', { id });

export const getBracketMatchups = (id) =>
  rpc('bracket.v1.BracketService', 'GetBracketMatchups', { id });

export const getBracketSummary = (id, viewerId) => {
  const body = { id };
  if (viewerId) body.viewer_id = viewerId;
  return rpc('bracket.v1.BracketService', 'GetBracketSummary', body);
};

export const attachMatchupToBracket = (bracketId, data) =>
  rpc('bracket.v1.BracketService', 'AttachMatchup', { bracket_id: bracketId, ...data });

export const updateBracket = (id, data) =>
  rpc('bracket.v1.BracketService', 'UpdateBracket', { id, ...data });

export const advanceBracket = (id) =>
  rpc('bracket.v1.BracketService', 'AdvanceBracket', { id });

export const archiveBracket = (id) =>
  rpc('bracket.v1.BracketService', 'UpdateBracket', { id, status: 'archived' });

export const deleteBracket = (id) =>
  rpc('bracket.v1.BracketService', 'DeleteBracket', { id });

export const getPopularBrackets = () =>
  rpc('bracket.v1.BracketService', 'GetPopularBrackets', {});

export const resolveTieAndAdvance = (matchupId, winnerItemId) =>
  rpc('bracket.v1.BracketService', 'ResolveTieAndAdvance', {
    matchup_id: matchupId,
    winner_item_id: winnerItemId,
  });

// -----------------------------------------
// ADMIN
// -----------------------------------------
export const adminGetUsers = (params = {}) =>
  rpc('admin.v1.AdminService', 'ListUsers', params);

export const adminUpdateUserRole = (userId, data) =>
  rpc('admin.v1.AdminService', 'UpdateUserRole', { id: userId, ...data });

export const adminDeleteUser = (userId) =>
  rpc('admin.v1.AdminService', 'DeleteUser', { id: userId });

export const adminGetMatchups = (params = {}) =>
  rpc('admin.v1.AdminService', 'ListMatchups', params);

export const adminDeleteMatchup = (matchupId) =>
  rpc('admin.v1.AdminService', 'DeleteMatchup', { id: matchupId });

export const adminGetBrackets = (params = {}) =>
  rpc('admin.v1.AdminService', 'ListBrackets', params);

export const adminDeleteBracket = (bracketId) =>
  rpc('admin.v1.AdminService', 'DeleteBracket', { id: bracketId });

export default API;
