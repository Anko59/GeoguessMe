import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { isServiceWorkerControlled, registerServiceWorker } from './serviceWorker';

const originalServiceWorker = Object.getOwnPropertyDescriptor(navigator, 'serviceWorker');

function setServiceWorker(value: unknown): void {
    Object.defineProperty(navigator, 'serviceWorker', { configurable: true, value });
}

function resetServiceWorker(): void {
    if (originalServiceWorker) Object.defineProperty(navigator, 'serviceWorker', originalServiceWorker);
    else Reflect.deleteProperty(navigator, 'serviceWorker');
}

beforeEach(resetServiceWorker);
afterEach(resetServiceWorker);

describe('service worker registration', () => {
    it('returns unsupported when serviceWorker API is absent', async () => {
        expect(await registerServiceWorker()).toBe('unsupported');
        expect(isServiceWorkerControlled()).toBe(false);
    });

    it('registers at root scope and activates a waiting worker', async () => {
        const postMessage = vi.fn();
        const register = vi.fn().mockResolvedValue({ waiting: { postMessage } });
        setServiceWorker({ register, controller: {} });

        await expect(registerServiceWorker()).resolves.toBe('registered');
        expect(register).toHaveBeenCalledWith('/sw.js', { scope: '/' });
        expect(postMessage).toHaveBeenCalledWith('SKIP_WAITING');
        expect(isServiceWorkerControlled()).toBe(true);
    });

    it('reports registration failure without preventing application startup', async () => {
        setServiceWorker({ register: vi.fn().mockRejectedValue(new Error('registration failed')), controller: null });
        await expect(registerServiceWorker()).resolves.toBe('error');
    });
});

describe('public service worker notification navigation', () => {
    it('replaces an external notification target with the safe groups route', async () => {
        type WorkerEvent = {
            notification: { close: () => void; data: { url: string } };
            waitUntil: (promise: Promise<unknown>) => void;
        };
        const listeners = new Map<string, (event: WorkerEvent) => void>();
        const openWindow = vi.fn().mockResolvedValue(null);
        const worker = {
            location: { origin: 'https://app.example' },
            clients: { matchAll: vi.fn().mockResolvedValue([]), openWindow, claim: vi.fn() },
            registration: { showNotification: vi.fn() },
            addEventListener: (name: string, listener: (event: WorkerEvent) => void) => listeners.set(name, listener),
            skipWaiting: vi.fn(),
        };
        const cacheStorage = { keys: vi.fn().mockResolvedValue([]), delete: vi.fn() };
        const nodeProcess = (
            globalThis as unknown as {
                process: { getBuiltinModule(name: string): { readFileSync(path: string, encoding: string): string } };
            }
        ).process;
        const source = nodeProcess.getBuiltinModule('fs').readFileSync('/workspace/frontend/public/sw.js', 'utf8');
        new Function('self', 'caches', source)(worker, cacheStorage);

        let navigation: Promise<unknown> | undefined;
        listeners.get('notificationclick')?.({
            notification: { close: vi.fn(), data: { url: 'https://evil.example/phish' } },
            waitUntil: (promise: Promise<unknown>) => {
                navigation = promise;
            },
        });
        await navigation;

        expect(openWindow).toHaveBeenCalledWith('/groups');
        expect(openWindow).not.toHaveBeenCalledWith(expect.stringContaining('evil.example'));
    });
});
