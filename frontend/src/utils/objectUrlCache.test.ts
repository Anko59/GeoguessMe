import { describe, expect, it, vi } from 'vitest';
import { createObjectUrlStore } from './objectUrlCache';

describe('createObjectUrlStore', () => {
    it('deduplicates concurrent fetches for the same key', async () => {
        const store = createObjectUrlStore();
        const fetcher = vi.fn(() => Promise.resolve('blob:one'));
        const first = store.getOrFetch('a', fetcher);
        const second = store.getOrFetch('a', fetcher);
        expect(fetcher).toHaveBeenCalledTimes(1);
        await expect(first).resolves.toBe('blob:one');
        expect(await second).toBe('blob:one');
        expect(store.get('a')).toBe('blob:one');
    });

    it('starts a fresh fetch once the previous one settled', async () => {
        const store = createObjectUrlStore();
        const fetcher = vi.fn(() => Promise.resolve('blob:one'));
        await store.getOrFetch('a', fetcher);
        await store.getOrFetch('a', fetcher);
        expect(fetcher).toHaveBeenCalledTimes(2);
    });

    it('bust revokes blob URLs and drops the cached entry', async () => {
        const store = createObjectUrlStore();
        const revoke = vi.spyOn(URL, 'revokeObjectURL');
        await store.getOrFetch('a', () => Promise.resolve('blob:avatar'));
        store.bust('a');
        expect(revoke).toHaveBeenCalledTimes(1);
        expect(revoke).toHaveBeenCalledWith('blob:avatar');
        expect(store.get('a')).toBeUndefined();
    });

    it('bust does not revoke static fallback URLs', async () => {
        const store = createObjectUrlStore();
        const revoke = vi.spyOn(URL, 'revokeObjectURL');
        await store.getOrFetch('a', () => Promise.resolve('/logo.png'));
        store.bust('a');
        expect(revoke).not.toHaveBeenCalled();
        expect(store.get('a')).toBeUndefined();
    });

    it('busting while a fetch is in flight prevents the stale result from being stored', async () => {
        const store = createObjectUrlStore();
        const revoke = vi.spyOn(URL, 'revokeObjectURL');
        let resolveFetch!: (url: string) => void;
        const inflight = store.getOrFetch(
            'a',
            () =>
                new Promise<string>((done) => {
                    resolveFetch = done;
                }),
        );
        store.bust('a');
        resolveFetch('blob:stale');
        await expect(inflight).resolves.toBeUndefined();
        expect(store.get('a')).toBeUndefined();
        expect(revoke).toHaveBeenCalledWith('blob:stale');
    });

    it('does not let a stale fetch clear or replace a newer fetch', async () => {
        const store = createObjectUrlStore();
        const revoke = vi.spyOn(URL, 'revokeObjectURL');
        let resolveStale!: (url: string) => void;
        const stale = store.getOrFetch('a', () => new Promise<string>((done) => (resolveStale = done)));
        store.bust('a');
        const fresh = store.getOrFetch('a', () => Promise.resolve('blob:fresh'));

        await expect(fresh).resolves.toBe('blob:fresh');
        resolveStale('blob:stale');
        await expect(stale).resolves.toBeUndefined();
        expect(store.get('a')).toBe('blob:fresh');
        expect(revoke).toHaveBeenCalledWith('blob:stale');
    });
});
