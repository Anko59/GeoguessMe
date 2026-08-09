/**
 * In-memory store for object URLs keyed by a string id, shared by the avatar
 * and group-photo caches (the two frontend caches with identical lifecycle
 * semantics: fetch once per session, deduplicate in-flight fetches, and revoke
 * the blob URL on bust). Data caches (e.g. the leaderboard) have different
 * lifetimes and must NOT use this store.
 *
 * Ownership contract: the module that creates the store owns its URLs. A URL
 * set through {@link set} is revoked exactly once by {@link bust}; the store
 * never revokes URLs it does not hold.
 */
export interface ObjectUrlStore {
    get(key: string): string | undefined;
    set(key: string, url: string): void;
    /** Return the in-flight fetch for a key or start one, so concurrent
     *  renders share a single request per key. */
    getOrFetch(key: string, fetcher: () => Promise<string>): Promise<string>;
    /** Revoke (when a blob: URL) and drop the cached URL and any in-flight
     *  request for the key. */
    bust(key: string): void;
}

export function createObjectUrlStore(): ObjectUrlStore {
    const urls = new Map<string, string>();
    const inflight = new Map<string, Promise<string>>();

    return {
        get(key) {
            return urls.get(key);
        },
        set(key, url) {
            urls.set(key, url);
        },
        getOrFetch(key, fetcher) {
            const existing = inflight.get(key);
            if (existing) return existing;
            const request = fetcher().finally(() => inflight.delete(key));
            inflight.set(key, request);
            return request;
        },
        bust(key) {
            const url = urls.get(key);
            if (url?.startsWith('blob:')) URL.revokeObjectURL(url);
            urls.delete(key);
            inflight.delete(key);
        },
    };
}
