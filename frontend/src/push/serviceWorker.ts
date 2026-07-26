/**
 * Service worker registration for the GeoGuessMe PWA. The worker only handles
 * push delivery and is intentionally minimal; it never caches the SPA. This
 * module is safe to call on every load: if the worker is already registered it
 * is a no-op, and registration errors are swallowed so a broken worker can
 * never prevent the app from rendering.
 */
const SW_URL = '/sw.js';

export type ServiceWorkerReadyState = 'unsupported' | 'error' | 'registered';

export async function registerServiceWorker(): Promise<ServiceWorkerReadyState> {
    if (typeof navigator === 'undefined' || !('serviceWorker' in navigator)) {
        return 'unsupported';
    }
    try {
        const registration = await navigator.serviceWorker.register(SW_URL, { scope: '/' });
        // When an updated worker takes over, ask the waiting one to activate
        // immediately so push handling stays current without a manual reload.
        if (registration.waiting) {
            registration.waiting.postMessage('SKIP_WAITING');
        }
        return 'registered';
    } catch {
        return 'error';
    }
}

/** Returns true when a service worker controls the page (installed PWA context). */
export function isServiceWorkerControlled(): boolean {
    return typeof navigator !== 'undefined' && 'serviceWorker' in navigator && !!navigator.serviceWorker.controller;
}
