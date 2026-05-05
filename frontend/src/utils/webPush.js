import {
  getPushConfig,
  subscribePush,
  unsubscribePush,
} from '../services/api';

/*
 * Web Push helper — wraps the ugly browser bits (service-worker
 * registration, VAPID key conversion, permission flow) into three
 * friendly functions used by NotificationSettings:
 *
 *   getPushState()        — "granted" | "denied" | "default" | "unsupported"
 *   enablePush()          — registers SW, asks permission, subscribes
 *   disablePush()         — unsubscribes on client + server
 *
 * All three fail soft: unsupported browsers / denied permissions /
 * server misconfigurations return a clear state/error to the caller
 * rather than throwing into React's render path.
 */

// Service-worker file path (see frontend/public/sw.js).
const SW_PATH = '/sw.js';

// isSupported returns true when the browser has the APIs we need.
// iOS Safari got Web Push in 16.4; older versions fall back to the
// email digest + bell badge.
export function isPushSupported() {
  if (typeof window === 'undefined') return false;
  return (
    'serviceWorker' in navigator &&
    'PushManager' in window &&
    'Notification' in window
  );
}

// getPushState — what should the settings UI render right now?
//   "unsupported" → hide the toggle entirely (or show a "not available" hint)
//   "denied"      → tell the user to unblock in browser settings
//   "default"     → the button reads "Enable push"
//   "granted"     → the button reads "Disable push"; we may also want
//                   to confirm the server still has our subscription
export async function getPushState() {
  if (!isPushSupported()) return 'unsupported';
  const permission = Notification.permission;
  if (permission === 'denied') return 'denied';
  if (permission === 'default') return 'default';
  // Permission granted — check if a subscription actually exists on
  // this browser. Re-installs / cache clears invalidate the sub
  // without changing the permission state.
  try {
    const reg = await navigator.serviceWorker.getRegistration();
    if (!reg) return 'default';
    const sub = await reg.pushManager.getSubscription();
    return sub ? 'granted' : 'default';
  } catch {
    return 'default';
  }
}

// enablePush: full opt-in flow. Returns the new PushSubscription
// object, or throws an Error with a UI-friendly message.
export async function enablePush() {
  if (!isPushSupported()) {
    throw new Error('Push notifications are not supported in this browser.');
  }

  const { data } = await getPushConfig();
  const vapidKey = data?.vapid_public_key;
  if (!vapidKey) {
    throw new Error('Push is not configured on this server.');
  }

  // Register (or re-use) the service worker at the root scope. A
  // re-registration with the same URL is a cheap no-op.
  const reg = await navigator.serviceWorker.register(SW_PATH);
  await navigator.serviceWorker.ready;

  // Ask for permission if we don't have it yet. The browser hides
  // this dialog if permission is already granted.
  let permission = Notification.permission;
  if (permission === 'default') {
    permission = await Notification.requestPermission();
  }
  if (permission !== 'granted') {
    throw new Error('Notification permission was not granted.');
  }

  // Re-use an existing subscription when available so we don't churn
  // the server's row.
  let sub = await reg.pushManager.getSubscription();
  if (!sub) {
    sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(vapidKey),
    });
  }

  // Hand the subscription to the server. Keys are base64url; extract
  // them from the JSON view of the subscription. platform is set
  // explicitly so the server doesn't have to guess when someday a
  // mobile-web client also calls this flow.
  const j = sub.toJSON();
  await subscribePush({
    platform: 'web',
    endpoint: j.endpoint,
    p256dhKey: j.keys?.p256dh,
    authKey: j.keys?.auth,
    userAgent: navigator.userAgent,
  });

  return sub;
}

// disablePush: unsubscribe on the server + cancel the browser-side
// subscription. Idempotent — calling when nothing is subscribed is a
// no-op.
export async function disablePush() {
  if (!isPushSupported()) return;
  const reg = await navigator.serviceWorker.getRegistration();
  if (!reg) return;
  const sub = await reg.pushManager.getSubscription();
  if (!sub) return;
  try {
    await unsubscribePush(sub.endpoint);
  } catch (err) {
    // Server unsubscribe failed — still try to cancel the browser
    // subscription so the user's intent is respected locally.
    console.warn('Server unsubscribe failed', err);
  }
  await sub.unsubscribe();
}

// urlBase64ToUint8Array converts the VAPID public key from its
// URL-safe base64 form (what the server sends) into the raw byte
// array pushManager.subscribe wants. Standard conversion per the
// Web Push examples.
function urlBase64ToUint8Array(base64String) {
  const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
  const raw = window.atob(base64);
  const output = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; ++i) output[i] = raw.charCodeAt(i);
  return output;
}
