/*
 * GeoGuessMe service worker.
 *
 * Scope is intentionally narrow: it exists to receive Web Push events and show
 * notifications, and to keep itself up to date. It deliberately does NOT cache
 * the SPA shell or hashed assets, because the app updates frequently and stale
 * cached bundles would break deployed versions. Media and data stay behind the
 * authenticated same-origin API, which is unaffected by this worker.
 */

const CACHE_VERSION = 'geoguessme-sw-v1';

// Notification actions open the deep link the backend sent in the payload.
function openTargetUrl(targetUrl) {
    if (targetUrl) {
        return self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
            for (const client of windowClients) {
                if (client.url.includes(targetUrl) && 'focus' in client) {
                    return client.focus();
                }
            }
            if (self.clients.openWindow) {
                return self.clients.openWindow(targetUrl);
            }
            return null;
        });
    }
    return self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
        for (const client of windowClients) {
            if ('focus' in client) {
                return client.focus();
            }
        }
        return null;
    });
}

self.addEventListener('install', (event) => {
    event.waitUntil(self.skipWaiting());
});

self.addEventListener('activate', (event) => {
    event.waitUntil(
        Promise.all([
            self.clients.claim(),
            caches.keys().then((keys) => {
                return Promise.all(keys.filter((key) => key !== CACHE_VERSION).map((key) => caches.delete(key)));
            }),
        ]),
    );
});

self.addEventListener('push', (event) => {
    let payload = { title: 'GeoGuessMe', body: 'You have a new update', url: '/groups', tag: 'geoguessme' };
    if (event.data) {
        try {
            payload = { ...payload, ...event.data.json() };
        } catch {
            payload.body = event.data.text() || payload.body;
        }
    }
    const options = {
        body: payload.body,
        tag: payload.tag,
        renotify: true,
        data: { url: payload.url || '/groups' },
        badge: payload.badge,
        icon: '/icons/icon-192.png',
        vibrate: [60, 30, 60],
    };
    event.waitUntil(self.registration.showNotification(payload.title || 'GeoGuessMe', options));
});

self.addEventListener('notificationclick', (event) => {
    event.notification.close();
    const targetUrl = event.notification.data && event.notification.data.url;
    event.waitUntil(openTargetUrl(targetUrl));
});

// Browsers can rotate or expire a subscription transparently. Re-establish it
// by asking an open client (which holds the access token) to resubscribe; the
// page-side push manager then re-POSTs the fresh subscription.
self.addEventListener('pushsubscriptionchange', (event) => {
    event.waitUntil(
        self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((windowClients) => {
            for (const client of windowClients) {
                client.postMessage({ type: 'PUSH_SUBSCRIPTION_CHANGE' });
            }
        }),
    );
});

// Allow the page to trigger update/activate flow without a reload.
self.addEventListener('message', (event) => {
    if (event.data === 'SKIP_WAITING') {
        self.skipWaiting();
    }
});
