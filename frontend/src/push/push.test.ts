import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const api = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), delete: vi.fn() }));

vi.mock('../api', () => ({ default: api }));

import {
    getActiveSubscription,
    getVapidPublicKey,
    isPushSupported,
    pushPermissionState,
    subscribePushNotifications,
    syncPushSubscription,
    unsubscribePushNotifications,
    urlB64ToUint8Array,
} from './push';

const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, 'serviceWorker');
const originalPushManager = Object.getOwnPropertyDescriptor(window, 'PushManager');
const originalNotification = Object.getOwnPropertyDescriptor(window, 'Notification');

type BrowserMocks = {
    subscription: PushSubscription;
    getSubscription: ReturnType<typeof vi.fn>;
    subscribe: ReturnType<typeof vi.fn>;
    requestPermission: ReturnType<typeof vi.fn>;
};

function mockSubscription(
    endpoint = 'https://push.example/subscription',
    applicationServerKey: BufferSource | null = null,
): PushSubscription {
    return {
        endpoint,
        expirationTime: null,
        options: { applicationServerKey },
        toJSON: () => ({ endpoint, keys: { p256dh: 'public-key', auth: 'auth-key' } }),
        unsubscribe: vi.fn().mockResolvedValue(true),
    } as unknown as PushSubscription;
}

function installPushBrowser(
    options: {
        permission?: NotificationPermission;
        permissionResponse?: NotificationPermission;
        subscription?: PushSubscription | null;
        nextSubscription?: PushSubscription;
    } = {},
): BrowserMocks {
    const subscription = options.subscription ?? mockSubscription();
    const getSubscription = vi.fn().mockResolvedValue(options.subscription === null ? null : subscription);
    const subscribe = vi.fn().mockResolvedValue(options.nextSubscription ?? subscription);
    const requestPermission = vi.fn().mockResolvedValue(options.permissionResponse ?? options.permission ?? 'granted');
    Object.defineProperty(navigator, 'serviceWorker', {
        configurable: true,
        value: { ready: Promise.resolve({ pushManager: { getSubscription, subscribe } }) },
    });
    Object.defineProperty(window, 'PushManager', { configurable: true, value: class PushManager {} });
    Object.defineProperty(window, 'Notification', {
        configurable: true,
        value: { permission: options.permission ?? 'granted', requestPermission },
    });
    return { subscription, getSubscription, subscribe, requestPermission };
}

function removePushBrowser(): void {
    if (originalServiceWorker) Object.defineProperty(navigator, 'serviceWorker', originalServiceWorker);
    else Reflect.deleteProperty(navigator, 'serviceWorker');
    if (originalPushManager) Object.defineProperty(window, 'PushManager', originalPushManager);
    else Reflect.deleteProperty(window, 'PushManager');
    if (originalNotification) Object.defineProperty(window, 'Notification', originalNotification);
    else Reflect.deleteProperty(window, 'Notification');
}

beforeEach(() => {
    api.get.mockReset();
    api.post.mockReset();
    api.delete.mockReset();
    removePushBrowser();
});

afterEach(removePushBrowser);

describe('browser capability helpers', () => {
    it('decodes a known base64url value', () => {
        expect(urlB64ToUint8Array('VGVzdA')).toEqual(new Uint8Array([84, 101, 115, 116]));
    });

    it('reports unsupported until every required browser API is present', () => {
        expect(isPushSupported()).toBe(false);
        installPushBrowser();
        expect(isPushSupported()).toBe(true);
        expect(pushPermissionState()).toBe('granted');
    });

    it('normalizes denied and default notification permission', () => {
        installPushBrowser({ permission: 'denied' });
        expect(pushPermissionState()).toBe('denied');
        installPushBrowser({ permission: 'default' });
        expect(pushPermissionState()).toBe('default');
    });
});

describe('VAPID and subscription lifecycle', () => {
    it('returns null when Push is disabled by the backend', async () => {
        api.get.mockRejectedValue(new Error('disabled'));
        await expect(getVapidPublicKey()).resolves.toBeNull();
        api.get.mockResolvedValue({ data: { public_key: '' } });
        await expect(getVapidPublicKey()).resolves.toBeNull();
    });

    it('returns the configured backend VAPID public key', async () => {
        api.get.mockResolvedValue({ data: { public_key: 'VGVzdA' } });
        await expect(getVapidPublicKey()).resolves.toBe('VGVzdA');
        expect(api.get).toHaveBeenCalledWith('/push/vapid-public-key');
    });

    it('subscribes after permission and persists a new browser subscription', async () => {
        const nextSubscription = mockSubscription('https://push.example/new');
        const browser = installPushBrowser({
            permission: 'default',
            permissionResponse: 'granted',
            subscription: null,
            nextSubscription,
        });
        api.get.mockResolvedValue({ data: { public_key: 'VGVzdA' } });
        api.post.mockResolvedValue({});

        await expect(subscribePushNotifications()).resolves.toBe(nextSubscription);
        expect(browser.requestPermission).toHaveBeenCalledOnce();
        expect(browser.subscribe).toHaveBeenCalledWith({
            userVisibleOnly: true,
            applicationServerKey: new Uint8Array([84, 101, 115, 116]),
        });
        expect(api.post).toHaveBeenCalledWith('/push/subscribe', {
            endpoint: 'https://push.example/new',
            keys: { p256dh: 'public-key', auth: 'auth-key' },
        });
    });

    it('does not subscribe when permission is denied or VAPID is disabled', async () => {
        const denied = installPushBrowser({ permission: 'denied' });
        await expect(subscribePushNotifications()).resolves.toBeNull();
        expect(denied.getSubscription).not.toHaveBeenCalled();

        const browser = installPushBrowser({ subscription: null });
        api.get.mockRejectedValue(new Error('disabled'));
        await expect(subscribePushNotifications()).resolves.toBeNull();
        expect(browser.requestPermission).not.toHaveBeenCalled();
        expect(browser.subscribe).not.toHaveBeenCalled();
    });

    it('creates a subscription when permission was granted before the app was installed', async () => {
        const nextSubscription = mockSubscription('https://push.example/installed');
        const browser = installPushBrowser({ subscription: null, nextSubscription });
        api.get.mockResolvedValue({ data: { public_key: 'VGVzdA' } });
        api.post.mockResolvedValue({});

        await expect(syncPushSubscription()).resolves.toBe(nextSubscription);
        expect(browser.requestPermission).not.toHaveBeenCalled();
        expect(browser.subscribe).toHaveBeenCalledOnce();
        expect(api.post).toHaveBeenCalledWith('/push/subscribe', expect.any(Object));
    });

    it('reconciles an existing subscription and removes one locally', async () => {
        const browser = installPushBrowser({
            subscription: mockSubscription('https://push.example/subscription', new Uint8Array([84, 101, 115, 116])),
        });
        api.get.mockResolvedValue({ data: { public_key: 'VGVzdA' } });
        api.post.mockResolvedValue({});
        api.delete.mockResolvedValue({});

        await expect(getActiveSubscription()).resolves.toBe(browser.subscription);
        await expect(syncPushSubscription()).resolves.toBe(browser.subscription);
        expect(api.post).toHaveBeenCalledWith('/push/subscribe', expect.any(Object));
        await expect(unsubscribePushNotifications()).resolves.toBe(true);
        expect(browser.subscription.unsubscribe).toHaveBeenCalledOnce();
        expect(api.delete).toHaveBeenCalledWith('/push/unsubscribe', {
            data: { endpoint: 'https://push.example/subscription' },
        });
    });

    it('replaces expired or VAPID-rotated subscriptions and tolerates backend unsubscribe failure', async () => {
        const expired = mockSubscription();
        Object.defineProperty(expired, 'expirationTime', { value: 0 });
        const replacement = mockSubscription('https://push.example/replacement');
        const browser = installPushBrowser({ subscription: expired, nextSubscription: replacement });
        api.get.mockResolvedValue({ data: { public_key: 'VGVzdA' } });
        api.post.mockResolvedValue({});
        api.delete.mockRejectedValue(new Error('offline'));

        await expect(syncPushSubscription()).resolves.toBe(replacement);
        expect(expired.unsubscribe).toHaveBeenCalledOnce();
        expect(browser.subscribe).toHaveBeenCalledOnce();
        vi.mocked(expired.unsubscribe).mockClear();
        await expect(unsubscribePushNotifications()).resolves.toBe(true);
        expect(expired.unsubscribe).toHaveBeenCalledOnce();
    });
});
