/**
 * Web Push subscription lifecycle for GeoGuessMe. Handles VAPID public key
 * fetch, permission requests, PushManager.subscribe, and backend sync. No
 * Firebase or native wrapper is used: subscriptions are stored by our backend
 * and delivered via standard push services.
 */
import api from '../api';

export type PushSubscriptionState = 'unsupported' | 'denied' | 'default' | 'granted';

interface BackendSubscription {
    endpoint: string;
    keys: { p256dh: string; auth: string };
}

interface VapidKeyResponse {
    public_key: string;
}

/** Whether the current browser can use Web Push at all. */
export function isPushSupported(): boolean {
    return (
        typeof window !== 'undefined' &&
        'serviceWorker' in navigator &&
        'PushManager' in window &&
        'Notification' in window
    );
}

/** Current notification permission as a normalized state. */
export function pushPermissionState(): PushSubscriptionState {
    if (!('Notification' in window)) {
        return 'unsupported';
    }
    switch (Notification.permission) {
        case 'granted':
            return 'granted';
        case 'denied':
            return 'denied';
        default:
            return 'default';
    }
}

/** Decode the unpadded base64url VAPID public key for PushManager.subscribe. */
export function urlB64ToUint8Array(base64String: string): Uint8Array {
    const padding = '='.repeat((4 - (base64String.length % 4)) % 4);
    const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/');
    const raw = atob(base64);
    const output = new Uint8Array(raw.length);
    for (let i = 0; i < raw.length; i++) {
        output[i] = raw.charCodeAt(i);
    }
    return output;
}

/** Fetch the configured VAPID public key from the backend, or null if disabled. */
export async function getVapidPublicKey(): Promise<string | null> {
    try {
        const response = await api.get<VapidKeyResponse>('/push/vapid-public-key');
        return response.data.public_key || null;
    } catch {
        return null;
    }
}

function toBackendSubscription(subscription: PushSubscription): BackendSubscription {
    const json = subscription.toJSON();
    if (!json.endpoint || !json.keys?.p256dh || !json.keys?.auth) {
        throw new Error('Push subscription is missing required keys');
    }
    return { endpoint: json.endpoint, keys: { p256dh: json.keys.p256dh, auth: json.keys.auth } };
}

async function postSubscriptionToBackend(subscription: PushSubscription): Promise<void> {
    await api.post('/push/subscribe', toBackendSubscription(subscription));
}

async function removeSubscriptionFromBackend(endpoint: string): Promise<void> {
    await api.delete('/push/unsubscribe', { data: { endpoint } });
}

/** The active PushSubscription for the controlled service worker, if any. */
export async function getActiveSubscription(): Promise<PushSubscription | null> {
    if (!isPushSupported()) {
        return null;
    }
    const registration = await navigator.serviceWorker.ready;
    return registration.pushManager.getSubscription();
}

/**
 * Subscribe to push notifications: request permission, create a browser
 * subscription bound to the VAPID key, and persist it with the backend. Returns
 * the subscription on success or null if the user denied permission or push is
 * unavailable.
 */
export async function subscribePushNotifications(): Promise<PushSubscription | null> {
    if (!isPushSupported()) {
        return null;
    }
    if (Notification.permission !== 'granted') {
        const permission = await Notification.requestPermission();
        if (permission !== 'granted') {
            return null;
        }
    }
    const vapidPublicKey = await getVapidPublicKey();
    if (!vapidPublicKey) {
        return null;
    }
    const registration = await navigator.serviceWorker.ready;
    const existing = await registration.pushManager.getSubscription();
    if (existing) {
        await postSubscriptionToBackend(existing);
        return existing;
    }
    const subscription = await registration.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlB64ToUint8Array(vapidPublicKey) as BufferSource,
    });
    await postSubscriptionToBackend(subscription);
    return subscription;
}

/** Unsubscribe locally and remove the subscription from the backend. */
export async function unsubscribePushNotifications(): Promise<boolean> {
    if (!isPushSupported()) {
        return false;
    }
    const subscription = await getActiveSubscription();
    if (!subscription) {
        return false;
    }
    const endpoint = subscription.endpoint;
    const unsubscribed = await subscription.unsubscribe();
    try {
        await removeSubscriptionFromBackend(endpoint);
    } catch {
        // The browser already revoked the local subscription; a stale backend
        // row is harmless because the push service returns 410 on the next send
        // and the backend prunes it automatically.
    }
    return unsubscribed;
}

/**
 * Reconcile the local subscription with the backend. The browser can rotate or
 * expire a subscription silently; this resubscribes and re-POSTs when the active
 * subscription differs from a fresh one, keeping notifications working without
 * requiring the user to re-enable them. Safe to call on app load.
 */
export async function syncPushSubscription(): Promise<PushSubscription | null> {
    if (!isPushSupported() || Notification.permission !== 'granted') {
        return null;
    }
    const vapidPublicKey = await getVapidPublicKey();
    if (!vapidPublicKey) {
        return null;
    }
    const registration = await navigator.serviceWorker.ready;
    const existing = await registration.pushManager.getSubscription();
    if (!existing) {
        return null;
    }
    const applicationServerKey = urlB64ToUint8Array(vapidPublicKey) as BufferSource;
    // Resubscribe if the endpoint expired or the subscription is bound to an old
    // VAPID key. Permission was already granted, so no prompt appears.
    if (
        subscriptionExpired(existing) ||
        !sameApplicationServerKey(existing.options.applicationServerKey, applicationServerKey)
    ) {
        await existing.unsubscribe();
        const subscription = await registration.pushManager.subscribe({
            userVisibleOnly: true,
            applicationServerKey,
        });
        await postSubscriptionToBackend(subscription);
        return subscription;
    }
    await postSubscriptionToBackend(existing);
    return existing;
}

function subscriptionExpired(subscription: PushSubscription): boolean {
    if ('expired' in subscription) {
        return (subscription as unknown as { expired: boolean }).expired;
    }
    // Fallback for runtimes without the `expired` attribute: check
    // expirationTime against the current clock.
    if (subscription.expirationTime !== null) {
        return subscription.expirationTime <= Date.now();
    }
    return false;
}

function sameApplicationServerKey(a: PushSubscriptionOptionsInit['applicationServerKey'], b: BufferSource): boolean {
    if (!a) {
        return false;
    }
    let aBytes: Uint8Array;
    if (a instanceof ArrayBuffer) {
        aBytes = new Uint8Array(a);
    } else if (ArrayBuffer.isView(a)) {
        aBytes = new Uint8Array(a.buffer, a.byteOffset, a.byteLength);
    } else {
        return false;
    }
    const bBytes = bufferSourceToUint8Array(b);
    if (aBytes.length !== bBytes.length) {
        return false;
    }
    for (let i = 0; i < bBytes.length; i++) {
        if (aBytes[i] !== bBytes[i]) {
            return false;
        }
    }
    return true;
}

function bufferSourceToUint8Array(source: BufferSource): Uint8Array {
    if (source instanceof ArrayBuffer) {
        return new Uint8Array(source);
    }
    return new Uint8Array(source.buffer, source.byteOffset, source.byteLength);
}
