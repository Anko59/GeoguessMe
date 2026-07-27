import { useEffect, useState } from 'react';
import api from '../../api';

// In-memory cache of resolved custom-avatar object URLs, keyed by user id. The
// cache is intentionally module-scoped so avatars are fetched at most once per
// session no matter how many components render them.
const cache = new Map<string, string>();
const inflight = new Map<string, Promise<string | undefined>>();
const fallbackAvatarPath = '/avatars/avatar.png';

/** Whether the avatar string represents a user-uploaded photo. */
export function isCustomAvatar(avatar?: string): boolean {
    return avatar === 'custom';
}

/** Drop the cached object URL so a fresh upload is reflected immediately. */
export function bustAvatarCache(userID: string): void {
    const url = cache.get(userID);
    if (url) {
        URL.revokeObjectURL(url);
        cache.delete(userID);
    }
    inflight.delete(userID);
}

function fetchAvatar(userID: string): Promise<string | undefined> {
    return api
        .get(`/users/${userID}/avatar`, { responseType: 'blob' })
        .then((res) => {
            const objectURL = URL.createObjectURL(res.data as Blob);
            cache.set(userID, objectURL);
            return objectURL;
        })
        .catch(() => fallbackAvatarPath);
}

/**
 * Resolve the avatar source for a user. Default avatars resolve to a static
 * path with no network call; custom avatars are fetched once as an
 * authenticated blob and reused for the rest of the session.
 */
export function useAvatarUrl(userID: string, avatar?: string): string | undefined {
    const [url, setUrl] = useState<string | undefined>(() => cache.get(userID));
    // Adjust cached state when the user id changes without a remount, using the
    // React-recommended store-previous-value pattern (no effect setState).
    const [prevUserID, setPrevUserID] = useState(userID);
    if (userID !== prevUserID) {
        setPrevUserID(userID);
        setUrl(cache.get(userID));
    }

    useEffect(() => {
        if (!isCustomAvatar(avatar) || cache.get(userID)) {
            return;
        }
        let cancelled = false;
        const promise = inflight.get(userID) ?? fetchAvatar(userID);
        if (!inflight.has(userID)) {
            inflight.set(userID, promise);
        }
        promise.then((result) => {
            inflight.delete(userID);
            if (!cancelled) {
                setUrl(result);
            }
        });
        return () => {
            cancelled = true;
        };
    }, [userID, avatar]);

    if (!isCustomAvatar(avatar)) {
        return `/avatars/${avatar || 'avatar.png'}`;
    }
    return url;
}
