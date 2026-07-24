import { registerServiceWorker, isServiceWorkerControlled } from './serviceWorker';

describe('registerServiceWorker', () => {
    it('returns unsupported when serviceWorker API is absent', async () => {
        expect(await registerServiceWorker()).toBe('unsupported');
    });
});

describe('isServiceWorkerControlled', () => {
    it('returns false when serviceWorker is absent', () => {
        expect(isServiceWorkerControlled()).toBe(false);
    });
});
