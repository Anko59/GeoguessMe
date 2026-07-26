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
