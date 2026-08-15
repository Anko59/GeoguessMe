import { useEffect, useState } from 'react';
import api from '../../api';
import { createObjectUrlStore } from '../../utils/objectUrlCache';

// Session-scoped cache of private group-photo URLs, keyed by group id, so the
// photo is fetched at most once per session.
const store = createObjectUrlStore();
const fallbackGroupPhoto = '/logo.png';

function fetchGroupPhoto(groupID: string): Promise<string> {
    return api
        .get('/group/photo', { params: { group_id: groupID }, responseType: 'blob' })
        .then((response) => URL.createObjectURL(response.data as Blob))
        .catch(() => fallbackGroupPhoto);
}

export function bustGroupPhotoCache(groupID: string): void {
    store.bust(groupID);
}

/** Resolve a private group photo, falling back to the app logo when unset. */
export function useGroupPhotoUrl(groupID: string, refreshKey = 0): string {
    const [url, setUrl] = useState(() => store.get(groupID) ?? fallbackGroupPhoto);
    const [previousGroupID, setPreviousGroupID] = useState(groupID);
    if (groupID !== previousGroupID) {
        setPreviousGroupID(groupID);
        setUrl(store.get(groupID) ?? fallbackGroupPhoto);
    }

    useEffect(() => {
        let cancelled = false;
        if (!groupID) {
            return () => {
                cancelled = true;
            };
        }
        if (store.get(groupID)) {
            return () => {
                cancelled = true;
            };
        }
        store
            .getOrFetch(groupID, () => fetchGroupPhoto(groupID))
            .then((result) => {
                if (!cancelled && result) setUrl(result);
            });
        return () => {
            cancelled = true;
        };
    }, [groupID, refreshKey]);

    return url;
}
