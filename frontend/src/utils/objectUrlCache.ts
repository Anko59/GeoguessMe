/**
 * In-memory store for object URLs keyed by a string id, shared by the avatar
 * and group-photo caches (the two frontend caches with identical lifecycle
 * semantics: fetch once per session, deduplicate in-flight fetches, and revoke
 * the blob URL on bust). Data caches (e.g. the leaderboard) have different
 * lifetimes and must NOT use this store.
 *
 * Ownership contract: getOrFetch stores successful URLs itself, rejects stale
 * generations after a bust, and revokes owned blob URLs on replacement or
 * bust. Fetchers must return URLs without caching them independently.
 */
export interface ObjectUrlStore {
    get(key: string): string | undefined;
    /** Return the in-flight fetch for a key or start one, so concurrent
     *  renders share a single request per key. */
    getOrFetch(key: string, fetcher: () => Promise<string>): Promise<string | undefined>;
    /** Revoke (when a blob: URL) and drop the cached URL and any in-flight
     *  request for the key. */
    bust(key: string): void;
}

export function createObjectUrlStore(): ObjectUrlStore {
    const urls = new Map<string, string>();
    const inflight = new Map<string, Promise<string | undefined>>();
    const generations = new Map<string, number>();

    const revoke = (url: string | undefined): void => {
        if (url?.startsWith('blob:')) URL.revokeObjectURL(url);
    };

    return {
        get(key) {
            return urls.get(key);
        },
        getOrFetch(key, fetcher) {
            const existing = inflight.get(key);
            if (existing) return existing;
            const generation = generations.get(key) ?? 0;
            const request = fetcher()
                .then((url) => {
                    if ((generations.get(key) ?? 0) !== generation) {
                        revoke(url);
                        return undefined;
                    }
                    const previous = urls.get(key);
                    if (previous !== url) revoke(previous);
                    urls.set(key, url);
                    return url;
                })
                .finally(() => {
                    if (inflight.get(key) === request) inflight.delete(key);
                });
            inflight.set(key, request);
            return request;
        },
        bust(key) {
            generations.set(key, (generations.get(key) ?? 0) + 1);
            revoke(urls.get(key));
            urls.delete(key);
            inflight.delete(key);
        },
    };
}
