// frontend/src/services/api.js
import axios from "axios";
import { peekAnonId, getOrCreateAnonId } from "../utils/anonId";

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
  } else {
    // Anonymous request — attach the X-Anon-Id header IFF an anon
    // UUID has already been minted (i.e. the user has voted at
    // least once). peekAnonId() never generates as a side effect,
    // so passive reads (home feed, matchup detail) don't pollute
    // localStorage. The signup endpoint reads this header to
    // migrate anon votes into the new account.
    const anonId = peekAnonId();
    if (anonId) {
      config.headers['X-Anon-Id'] = anonId;
    }
  }
  config.headers['Connect-Protocol-Version'] = '1';
  return config;
});

// Helper: call a ConnectRPC method
const rpc = (service, method, body = {}) =>
  API.post(`/${service}/${method}`, body);

// ---------------------------------------------------------------
// Refresh-token interceptor
// ---------------------------------------------------------------
// Access tokens expire every 15 minutes. Instead of bothering the
// user with a re-login, any 401 triggers a silent Refresh call with
// the stored refresh token; the original request is retried once the
// new access token lands.
//
// Concurrency: N requests failing in parallel (typical on a screen
// that fires a handful of reads at once) should produce exactly ONE
// Refresh call — we coalesce them onto a shared promise. If Refresh
// itself 401s, we clear auth and redirect to /login.
let inFlightRefresh = null;

// The Refresh RPC is the one 401 we NEVER retry — retrying it would
// be an infinite loop. Comparing against the full request URL would
// be fragile (relative vs absolute); the path suffix is stable.
const REFRESH_URL = '/auth.v1.AuthService/Refresh';

const clearAuthStorage = () => {
  localStorage.removeItem('token');
  localStorage.removeItem('refresh_token');
  localStorage.removeItem('userId');
  localStorage.removeItem('username');
  localStorage.removeItem('isAdmin');
  delete API.defaults.headers.Authorization;
};

// performRefresh is exported for tests; production code hits this via
// the interceptor below.
export const performRefresh = async () => {
  const refreshToken = localStorage.getItem('refresh_token');
  if (!refreshToken) throw new Error('no refresh token stored');

  const res = await API.post(REFRESH_URL, { refresh_token: refreshToken });
  const payload = res?.data || {};
  if (!payload.token || !payload.refresh_token) {
    throw new Error('malformed refresh response');
  }
  localStorage.setItem('token', payload.token);
  localStorage.setItem('refresh_token', payload.refresh_token);
  API.defaults.headers.Authorization = `Bearer ${payload.token}`;
  return payload.token;
};

API.interceptors.response.use(
  (response) => response,
  async (error) => {
    const original = error.config;
    const status = error.response?.status;

    // Capture server-side failures (5xx) to Sentry with a breadcrumb
    // trail. 4xx deliberately NOT captured — client-side errors like
    // rate-limit or validation are expected and shouldn't consume the
    // error budget. Dynamic import keeps api.js independent of the
    // Sentry module load order.
    if (status >= 500 && status < 600) {
      import('../sentry').then(({ Sentry }) => {
        Sentry.captureException(error, {
          tags: {
            rpc_path: original?.url,
            status: String(status),
          },
        });
      }).catch(() => { /* module unavailable; swallow */ });
    }

    // Not a 401, or the failing request IS /Refresh itself → bail out.
    if (status !== 401 || !original || original.url?.endsWith(REFRESH_URL)) {
      return Promise.reject(error);
    }
    // Guard against retrying the same request twice.
    if (original._retriedAfterRefresh) {
      return Promise.reject(error);
    }
    // No refresh token locally → nothing we can do; surface the 401.
    if (!localStorage.getItem('refresh_token')) {
      return Promise.reject(error);
    }

    try {
      if (!inFlightRefresh) {
        inFlightRefresh = performRefresh().finally(() => {
          inFlightRefresh = null;
        });
      }
      const newAccess = await inFlightRefresh;
      original._retriedAfterRefresh = true;
      original.headers = original.headers || {};
      original.headers.Authorization = `Bearer ${newAccess}`;
      return API.request(original);
    } catch (refreshErr) {
      // Refresh itself failed (expired / revoked / theft-detected).
      // Clear local auth and send the user home. Tests / non-browser
      // contexts should tolerate the lack of window.location.
      clearAuthStorage();
      if (typeof window !== 'undefined' && window.location) {
        // Intentionally a hard navigation, not react-router —
        // api.js has no router reference and we want every in-flight
        // stateful screen torn down.
        window.location.href = '/login';
      }
      return Promise.reject(refreshErr);
    }
  },
);

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
  if (payload.refresh_token) {
    localStorage.setItem('refresh_token', payload.refresh_token);
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

// Logout is "hey server, please revoke this device's refresh token."
// It's best-effort — the caller should still clear local storage
// afterward so the UX is unambiguous if the server fails.
export const logout = (refreshToken) =>
  rpc('auth.v1.AuthService', 'Logout', { refresh_token: refreshToken });

// LogoutAll revokes every active refresh token for the authed user
// across every device they've signed in on. Useful for "I lost my
// phone" flows; no UI surface yet but the RPC is here when we need it.
export const logoutAll = () =>
  rpc('auth.v1.AuthService', 'LogoutAll', {});

// signOutLocally clears every auth-related key + the axios default
// Authorization header. Exported so the UI layer's logout button can
// call this + logout(...) in the same handler regardless of server
// success.
export const signOutLocally = () => {
  clearAuthStorage();
};

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

// Self-serve account deletion. Requires the caller's current password
// as a confirmation signal; the server soft-deletes (blanking public
// fields immediately) and returns the scheduled hard-delete timestamp
// that the UI shows the user.
export const deleteMyAccount = (password, reason) =>
  rpc('user.v1.UserService', 'DeleteMyAccount', {
    password,
    ...(reason ? { reason } : {}),
  });

// Email verification (soft-nudge). Request fires a fresh verification
// link to the signed-in user's email. Confirm is called from the link
// landing page with the token pulled from the URL.
export const requestEmailVerification = () =>
  rpc('auth.v1.AuthService', 'RequestEmailVerification', {});

export const confirmEmailVerification = (token) =>
  rpc('auth.v1.AuthService', 'ConfirmEmailVerification', { token });

// Content reporting. subjectType is one of:
//   "matchup" | "bracket" | "comment" | "bracket_comment" | "user"
// subjectId is the UUID public_id. reason is one of the closed-list
// values enforced by the server. reasonDetail is required when
// reason === "other".
export const reportContent = ({ subjectType, subjectId, reason, reasonDetail }) =>
  rpc('report.v1.ReportService', 'ReportContent', {
    subject_type: subjectType,
    subject_id: subjectId,
    reason,
    ...(reasonDetail ? { reason_detail: reasonDetail } : {}),
  });

// Notification preferences. Server enforces auth; UI only renders the
// settings section for the signed-in viewer.
export const getNotificationPreferences = () =>
  rpc('user.v1.UserService', 'GetNotificationPreferences', {});

export const updateNotificationPreferences = (prefs) =>
  rpc('user.v1.UserService', 'UpdateNotificationPreferences', { prefs });

// Per-item read state for persisted notifications. Stamps read_at on
// every unread row the viewer owns with occurred_at <= before. Omit
// `before` to use NOW() server-side (mark all currently visible).
export const markActivityRead = (before) =>
  rpc('activity.v1.ActivityService', 'MarkActivityRead', before ? { before } : {});

// Push config + subscription management. The RPC set serves web
// (VAPID) and future mobile (APNS/FCM) clients through a single
// platform-discriminated subscribe call. GetPushConfig returns the
// VAPID public key; native clients ignore that field.
export const getPushConfig = () =>
  rpc('activity.v1.ActivityService', 'GetPushConfig', {});

// subscribePush({ platform, endpoint, p256dhKey?, authKey?, userAgent? })
//   platform   "web" (default) | "ios" | "android"
//   endpoint   browser push URL (web) or device token (mobile)
//   p256dhKey  required when platform === "web"
//   authKey    required when platform === "web"
export const subscribePush = ({
  platform = 'web',
  endpoint,
  p256dhKey,
  authKey,
  userAgent,
}) =>
  rpc('activity.v1.ActivityService', 'SubscribePush', {
    platform,
    endpoint,
    ...(p256dhKey ? { p256dh_key: p256dhKey } : {}),
    ...(authKey ? { auth_key: authKey } : {}),
    ...(userAgent ? { user_agent: userAgent } : {}),
  });

export const unsubscribePush = (endpoint) =>
  rpc('activity.v1.ActivityService', 'UnsubscribePush', { endpoint });

// presignUpload — ask the server for an S3 PUT-scoped signed URL.
// Returns { upload_url, key, max_bytes, expires_in_seconds }. The
// client PUTs raw bytes to upload_url, then sends the `key` back to
// whichever commit RPC owns this `kind` (UpdateAvatar for avatars,
// CreateMatchup.upload_key for matchup covers).
export const presignUpload = (kind, contentType) =>
  rpc('upload.v1.UploadService', 'PresignUpload', {
    kind,
    content_type: contentType,
  });

// uploadViaPresign runs the three-step flow end-to-end: presign →
// PUT the raw file → return the key. Callers hand the key to the
// commit RPC (UpdateAvatar / CreateMatchup).
//
// Throws a plain Error with a UI-friendly message on:
//  - file too large (server-declared cap)
//  - S3 PUT rejection
//  - server-side presign failure
async function uploadViaPresign(kind, file) {
  const presignRes = await presignUpload(kind, file.type);
  const { upload_url, key, max_bytes } = presignRes?.data || {};
  if (!upload_url || !key) throw new Error('Presign failed.');

  const maxBytes = Number(max_bytes || 0);
  if (maxBytes > 0 && file.size > maxBytes) {
    throw new Error(`File is too large (max ${(maxBytes / 1024 / 1024).toFixed(1)} MB).`);
  }

  // Direct-to-S3 PUT. Content-Type must match what was presigned;
  // S3 rejects with SignatureDoesNotMatch otherwise. `fetch` is the
  // right primitive here — axios wraps headers we don't want to send
  // on a cross-origin S3 request.
  let putRes;
  try {
    putRes = await fetch(upload_url, {
      method: 'PUT',
      body: file,
      headers: { 'Content-Type': file.type },
    });
  } catch (netErr) {
    // fetch only rejects on network-level failures: DNS, TLS, CORS
    // preflight blocked, server unreachable. CORS is the most
    // common — the bucket policy needs to list the page's origin,
    // otherwise the browser cancels the request before the server
    // sees it. Surface the *actual* origin so the user knows what
    // to allow-list (the message used to hardcode localhost:3000,
    // which was wrong everywhere except local dev).
    const origin = (typeof window !== 'undefined' && window.location?.origin) || '<unknown origin>';
    console.error('S3 PUT network error', { upload_url, origin, message: netErr?.message, file_type: file.type, file_size: file.size });
    throw new Error(`S3 upload blocked by browser (likely CORS). Add ${origin} to the bucket's CORS allowlist. Underlying: ${netErr?.message || 'network error'}`);
  }
  if (!putRes.ok) {
    let body = '';
    try { body = (await putRes.text()).slice(0, 300); } catch { /* swallow */ }
    console.error('S3 PUT non-2xx', { status: putRes.status, body });
    throw new Error(`S3 upload failed: ${putRes.status}${body ? ` — ${body}` : ''}`);
  }

  return key;
}

export const updateUserAvatar = async (userId, file) => {
  const uploadKey = await uploadViaPresign('avatar', file);
  return rpc('user.v1.UserService', 'UpdateAvatar', {
    id: userId,
    upload_key: uploadKey,
  });
};

// Callers (e.g. CreateMatchup page) pass a File object when they want
// a cover image. This helper uploads it and returns the S3 key; if
// `file` is nullish it returns undefined so the caller can spread
// conditionally.
export const uploadMatchupCoverImage = async (file) => {
  if (!file) return undefined;
  return uploadViaPresign('matchup_cover', file);
};

// uploadMatchupItemImage — same pattern as uploadMatchupCoverImage but
// for per-contender thumbnails (kind="matchup_item", 2 MB cap). Returns
// the S3 key the backend expects on the matching commit RPC
// (CreateMatchup.items[].upload_key, AddItem.upload_key,
// UpdateItem.upload_key).
export const uploadMatchupItemImage = async (file) => {
  if (!file) return undefined;
  return uploadViaPresign('matchup_item', file);
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
// BLOCKS + MUTES
// -----------------------------------------
// Block is bidirectional: the target disappears from the viewer's
// feeds AND the viewer disappears from the target's feeds. Server
// also severs any existing follow edges both ways. Id accepts either
// a username or a public UUID.
export const blockUser = (id) =>
  rpc('user.v1.UserService', 'BlockUser', { id });

export const unblockUser = (id) =>
  rpc('user.v1.UserService', 'UnblockUser', { id });

export const listBlocks = (params = {}) =>
  rpc('user.v1.UserService', 'ListBlocks', params);

// Mute is one-way: the muted user's activity stops appearing in the
// muter's feed + push delivery, but follows + visibility remain in
// both directions.
export const muteUser = (id) =>
  rpc('user.v1.UserService', 'MuteUser', { id });

export const unmuteUser = (id) =>
  rpc('user.v1.UserService', 'UnmuteUser', { id });

export const listMutes = (params = {}) =>
  rpc('user.v1.UserService', 'ListMutes', params);

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

// createMatchup accepts:
//  - imageFile (a File) for the matchup cover
//  - items: [{ item, imageFile? }] — each item may carry an optional
//    File for its thumbnail (cycle 6c)
// Both upload paths run in parallel via Promise.all so a 4-item
// matchup with thumbnails on every item doesn't pay 5× sequential
// network round-trips. Failures bubble up as plain Errors with the
// uploadViaPresign messages; the caller surfaces those inline.
export const createMatchup = async (userId, data = {}) => {
  const { imageFile, items: rawItems, ...rest } = data;

  const coverPromise = uploadMatchupCoverImage(imageFile);
  const itemsPromise = Promise.all(
    (rawItems ?? []).map(async ({ imageFile: itemFile, ...itemRest }) => {
      const upload_key = await uploadMatchupItemImage(itemFile);
      return { ...itemRest, ...(upload_key ? { upload_key } : {}) };
    }),
  );

  const [upload_key, items] = await Promise.all([coverPromise, itemsPromise]);

  return rpc('matchup.v1.MatchupService', 'CreateMatchup', {
    user_id: userId,
    ...rest,
    items,
    ...(upload_key ? { upload_key } : {}),
  });
};

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
// Vote on an item. When the caller is signed in, the request is
// authenticated by the bearer token and the anon_id field is
// ignored. When the caller is anonymous, getOrCreateAnonId() mints
// a stable per-browser UUID on first vote and persists it in
// localStorage; subsequent calls reuse the same value so the
// server's per-anon-id 3-vote cap accumulates across the session.
export const incrementMatchupItemVotes = (id) => {
  const body = { id };
  if (!localStorage.getItem('token')) {
    body.anon_id = getOrCreateAnonId();
  }
  return rpc('matchup.v1.MatchupItemService', 'VoteItem', body);
};

// SkipMatchup — record that the viewer chose not to pick either
// contender. Skips are stored in matchup_votes alongside picks but
// with kind='skip' and a NULL item reference; they do NOT count
// toward the anon 3-vote cap. Anon callers cannot skip on bracket
// matchups (members-only, mirrors VoteItem). Server returns
// `{already_skipped: bool}` — the frontend uses that to distinguish
// "skip recorded just now" from "you'd already skipped this".
export const skipMatchup = (matchupId) => {
  const body = { matchup_id: matchupId };
  if (!localStorage.getItem('token')) {
    body.anon_id = getOrCreateAnonId();
  }
  return rpc('matchup.v1.MatchupItemService', 'SkipMatchup', body);
};

// GetAnonVoteStatus — returns {used, max}. Drives the "X of 3 free
// votes left" counter chip on the matchup feed. Only meaningful for
// anonymous callers; passes the existing anon UUID via peekAnonId
// (no side-effect generation).
export const getAnonVoteStatus = () => {
  const anonId = peekAnonId();
  return rpc('matchup.v1.MatchupItemService', 'GetAnonVoteStatus', {
    anon_id: anonId || '',
  });
};

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
// ACTIVITY (notifications-ish feed)
// -----------------------------------------
// Returns a merged feed of the viewer's recent activity: votes they
// cast, wins/losses, bracket progress, plus on-them events (someone
// voted/liked/commented on their content, new followers). Backed by
// derived queries server-side — no new write path.
export const getUserActivity = (userId, { limit, before } = {}) =>
  rpc('activity.v1.ActivityService', 'GetUserActivity', {
    user_id: userId,
    ...(limit ? { limit } : {}),
    ...(before ? { before } : {}),
  });

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

// -----------------------------------------
// ADMIN MODERATION QUEUE
// -----------------------------------------
// Reports queue. Pass `{ status: 'open' | 'resolved' }` to filter; omit
// to get open by default. Paginated via cursor.
export const adminListReports = (params = {}) =>
  rpc('admin.v1.AdminService', 'ListReports', params);

// Resolve a report with one of the four canonical actions. Pass
// ban_reason when resolution === "ban_user".
export const adminResolveReport = ({ reportId, resolution, notes, banReason }) =>
  rpc('admin.v1.AdminService', 'ResolveReport', {
    report_id: reportId,
    resolution,
    ...(notes ? { notes } : {}),
    ...(banReason ? { ban_reason: banReason } : {}),
  });

// Direct ban / unban. Reason is required for ban. Unban clears both
// deleted_at + banned_at so the user can log back in.
export const adminBanUser = (id, reason) =>
  rpc('admin.v1.AdminService', 'BanUser', { id, reason });

export const adminUnbanUser = (id) =>
  rpc('admin.v1.AdminService', 'UnbanUser', { id });

// -----------------------------------------
// Community service. v1 ships these endpoints; the rest of the
// surface (member listing, role mgmt, ban/unban, rules) lights up
// when the UI for those features lands.
// -----------------------------------------
export const createCommunity = (data) =>
  rpc('community.v1.CommunityService', 'CreateCommunity', {
    slug: data.slug,
    name: data.name,
    description: data.description || '',
    avatar_path: data.avatarPath || '',
    banner_path: data.bannerPath || '',
    tags: data.tags || [],
    privacy: data.privacy || 'public',
  });

export const getCommunity = (id) =>
  rpc('community.v1.CommunityService', 'GetCommunity', { id });

export const getCommunityBySlug = (slug) =>
  rpc('community.v1.CommunityService', 'GetCommunityBySlug', { slug });

export const listCommunities = (params = {}) =>
  rpc('community.v1.CommunityService', 'ListCommunities', {
    limit: params.limit ?? 20,
    cursor: params.cursor || '',
    tag: params.tag || '',
    query: params.query || '',
  });

// Slug availability — used by the create form for live feedback as
// the user types. Server returns { available, reason, suggestions }.
export const checkSlugAvailable = (slug) =>
  rpc('community.v1.CommunityService', 'CheckSlugAvailable', { slug });

export const joinCommunity = (id) =>
  rpc('community.v1.CommunityService', 'JoinCommunity', { id });

export const leaveCommunity = (id) =>
  rpc('community.v1.CommunityService', 'LeaveCommunity', { id });

export const getMyCommunityMembership = (communityId) =>
  rpc('community.v1.CommunityService', 'GetMyMembership', { community_id: communityId });

export const listCommunityMembers = (params) =>
  rpc('community.v1.CommunityService', 'ListMembers', {
    community_id: params.communityId,
    limit: params.limit ?? 50,
    cursor: params.cursor || '',
    role: params.role || '',
  });

export const updateCommunity = (id, data = {}) =>
  rpc('community.v1.CommunityService', 'UpdateCommunity', {
    id,
    ...(data.name !== undefined ? { name: data.name } : {}),
    ...(data.description !== undefined ? { description: data.description } : {}),
    ...(data.avatarPath !== undefined ? { avatar_path: data.avatarPath } : {}),
    ...(data.bannerPath !== undefined ? { banner_path: data.bannerPath } : {}),
    ...(data.tags !== undefined ? { tags: data.tags } : {}),
    ...(data.privacy !== undefined ? { privacy: data.privacy } : {}),
  });

export const deleteCommunity = (id) =>
  rpc('community.v1.CommunityService', 'DeleteCommunity', { id });

// Feed for a community page — matchups + brackets the community
// owns, sorted by created_at DESC. cursor is an RFC3339 timestamp
// returned by the previous page as next_cursor.
export const getCommunityFeed = (communityId, { limit = 20, cursor = '' } = {}) =>
  rpc('community.v1.CommunityService', 'GetCommunityFeed', {
    community_id: communityId,
    limit,
    cursor,
  });

export default API;
