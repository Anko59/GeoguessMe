import { useEffect, useState } from 'react';
import api from '../../api';

const cache = new Map<string, string>();
const inflight = new Map<string, Promise<string>>();
const fallbackGroupPhoto = '/logo.png';

function fetchGroupPhoto(groupID: string): Promise<string> {
    return api
        .get('/group/photo', { params: { group_id: groupID }, responseType: 'blob' })
        .then((response) => {
            const objectURL = URL.createObjectURL(response.data as Blob);
            cache.set(groupID, objectURL);
            return objectURL;
        })
        .catch(() => {
            cache.set(groupID, fallbackGroupPhoto);
            return fallbackGroupPhoto;
        });
}

export function bustGroupPhotoCache(groupID: string): void {
    const url = cache.get(groupID);
    if (url?.startsWith('blob:')) {
        URL.revokeObjectURL(url);
    }
    cache.delete(groupID);
    inflight.delete(groupID);
}

/** Resolve a private group photo, falling back to the app logo when unset. */
export function useGroupPhotoUrl(groupID: string, refreshKey = 0): string {
    const [url, setUrl] = useState(() => cache.get(groupID) ?? fallbackGroupPhoto);
    const [previousGroupID, setPreviousGroupID] = useState(groupID);
    if (groupID !== previousGroupID) {
        setPreviousGroupID(groupID);
        setUrl(cache.get(groupID) ?? fallbackGroupPhoto);
    }

    useEffect(() => {
        let cancelled = false;
        if (!groupID) {
            return () => {
                cancelled = true;
            };
        }
        const cached = cache.get(groupID);
        if (cached) {
            return () => {
                cancelled = true;
            };
        }
        const promise = inflight.get(groupID) ?? fetchGroupPhoto(groupID);
        inflight.set(groupID, promise);
        promise.then((result) => {
            inflight.delete(groupID);
            if (!cancelled) setUrl(result);
        });
        return () => {
            cancelled = true;
        };
    }, [groupID, refreshKey]);

    return url;
}
