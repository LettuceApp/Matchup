// Matchup service worker — handles Web Push events so native browser
// notifications can fire while the tab is backgrounded or closed.
//
// The SW is intentionally minimal: it ONLY cares about push events.
// No caching / offline behaviour; the React app owns that story.
//
// Payload contract (see api/cache/webpush.go PushPayload):
//   { title: string, body: string, url?: string, tag?: string }
// Unknown keys are ignored. Missing title falls through to a sensible
// default so a malformed push still shows SOMETHING rather than going
// silent.

self.addEventListener('install', (event) => {
  // Take over from any older SW immediately so users stop seeing stale
  // behaviour after a re-deploy.
  event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener('push', (event) => {
  let payload = { title: 'Matchup', body: 'New activity' };
  if (event.data) {
    try {
      const parsed = event.data.json();
      payload = { ...payload, ...parsed };
    } catch {
      // Fallback to text if the server ever ships non-JSON.
      try { payload.body = event.data.text() || payload.body; } catch {}
    }
  }

  const options = {
    body: payload.body,
    // `tag` collapses duplicates (multiple milestones in a short window
    // don't stack into a tray pile; the latest replaces the older).
    tag: payload.tag || undefined,
    data: { url: payload.url || '/' },
    // Icon/badge: use the public favicon so installs look native
    // without shipping extra assets.
    icon: '/favicon.ico',
    badge: '/favicon.ico',
  };

  event.waitUntil(self.registration.showNotification(payload.title, options));
});

// taggedPushURL appends utm_source=push (+ optional push_kind from the
// notification tag) to the URL so PostHog's analytics show pushes as
// a discrete acquisition channel and the React app can fire a
// `push_clicked` event on landing. Preserves any existing query
// string + hash on the URL.
const taggedPushURL = (rawURL, tag) => {
  try {
    const u = new URL(rawURL, self.registration.scope);
    u.searchParams.set('utm_source', 'push');
    if (tag) u.searchParams.set('push_kind', tag);
    return u.toString();
  } catch {
    return rawURL;
  }
};

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const rawURL = (event.notification.data && event.notification.data.url) || '/';
  const tag = event.notification.tag;
  const targetURL = taggedPushURL(rawURL, tag);

  event.waitUntil((async () => {
    // Prefer re-focusing an existing tab over opening a new one so we
    // don't spawn N Matchup windows for heavy notifiers.
    const windowClients = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    for (const client of windowClients) {
      if (client.url && client.url.startsWith(self.registration.scope)) {
        await client.focus();
        // Navigate the focused tab to the payload URL so the click
        // lands on the relevant page even if the tab was on a
        // different route.
        if ('navigate' in client) {
          try { await client.navigate(targetURL); } catch {}
        }
        return;
      }
    }
    await self.clients.openWindow(targetURL);
  })());
});
