import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({
    isAuthenticated: false,
    isPushSupported: vi.fn(() => false),
    registerServiceWorker: vi.fn(),
    syncPushSubscription: vi.fn(),
}));

vi.mock('../context/AuthContext', () => ({ useAuth: () => ({ isAuthenticated: mocks.isAuthenticated }) }));
vi.mock('./push', () => ({ isPushSupported: mocks.isPushSupported, syncPushSubscription: mocks.syncPushSubscription }));
vi.mock('./serviceWorker', () => ({ registerServiceWorker: mocks.registerServiceWorker }));

import { usePushBootstrap } from './usePushBootstrap';

const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, 'serviceWorker');
const originalNotification = Object.getOwnPropertyDescriptor(window, 'Notification');

function installServiceWorker(): { emit: (data: unknown) => void; remove: ReturnType<typeof vi.fn> } {
    let listener: ((event: MessageEvent) => void) | undefined;
    const add = vi.fn((_: string, callback: (event: MessageEvent) => void) => {
        listener = callback;
    });
    const remove = vi.fn();
    Object.defineProperty(navigator, 'serviceWorker', {
        configurable: true,
        value: { addEventListener: add, removeEventListener: remove },
    });
    Object.defineProperty(window, 'Notification', { configurable: true, value: { permission: 'granted' } });
    return { emit: (data) => listener?.({ data } as MessageEvent), remove };
}

function resetBrowser(): void {
    if (originalServiceWorker) Object.defineProperty(navigator, 'serviceWorker', originalServiceWorker);
    else Reflect.deleteProperty(navigator, 'serviceWorker');
    if (originalNotification) Object.defineProperty(window, 'Notification', originalNotification);
    else Reflect.deleteProperty(window, 'Notification');
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.isAuthenticated = false;
    mocks.isPushSupported.mockReturnValue(false);
    mocks.registerServiceWorker.mockResolvedValue('registered');
    mocks.syncPushSubscription.mockResolvedValue(null);
    resetBrowser();
});

afterEach(resetBrowser);

describe('usePushBootstrap', () => {
    it('registers the worker but avoids Push listeners in unsupported browsers', () => {
        renderHook(() => usePushBootstrap());
        expect(mocks.registerServiceWorker).toHaveBeenCalledOnce();
        expect(mocks.syncPushSubscription).not.toHaveBeenCalled();
    });

    it('syncs authenticated granted subscriptions and reacts to worker rotation messages', () => {
        mocks.isAuthenticated = true;
        mocks.isPushSupported.mockReturnValue(true);
        const worker = installServiceWorker();
        const { unmount } = renderHook(() => usePushBootstrap());

        expect(mocks.syncPushSubscription).toHaveBeenCalledOnce();
        act(() => worker.emit({ type: 'PUSH_SUBSCRIPTION_CHANGE' }));
        expect(mocks.syncPushSubscription).toHaveBeenCalledTimes(2);
        unmount();
        expect(worker.remove).toHaveBeenCalledWith('message', expect.any(Function));
    });
});
