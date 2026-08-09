import { useEffect, useState } from 'react';
import api from '../../api';
import { createObjectUrlStore } from '../../utils/objectUrlCache';

// Session-scoped cache of resolved custom-avatar object URLs, keyed by user
// id. The store is intentionally module-scoped so avatars are fetched at most
// once per session no matter how many components render them.
const store = createObjectUrlStore();
const fallbackAvatarPath = '/avatars/avatar.png';

/** Whether the avatar string represents a user-uploaded photo. */
export function isCustomAvatar(avatar?: string): boolean {
    return avatar === 'custom';
}

/** Drop the cached object URL so a fresh upload is reflected immediately. */
export function bustAvatarCache(userID: string): void {
    store.bust(userID);
}

function fetchAvatar(userID: string): Promise<string> {
    return api
        .get(`/users/${userID}/avatar`, { responseType: 'blob' })
        .then((res) => {
            const objectURL = URL.createObjectURL(res.data as Blob);
            store.set(userID, objectURL);
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
    const [url, setUrl] = useState<string | undefined>(() => store.get(userID));
    // Adjust cached state when the user id changes without a remount, using the
    // React-recommended store-previous-value pattern (no effect setState).
    const [prevUserID, setPrevUserID] = useState(userID);
    if (userID !== prevUserID) {
        setPrevUserID(userID);
        setUrl(store.get(userID));
    }

    useEffect(() => {
        if (!isCustomAvatar(avatar) || store.get(userID)) {
            return;
        }
        let cancelled = false;
        store
            .getOrFetch(userID, () => fetchAvatar(userID))
            .then((result) => {
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
