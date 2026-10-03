import { useEffect, useState, type ReactNode } from 'react';
import { clearAvatarCache } from '../components/common/avatarCache';
import { clearLeaderboardCache } from '../components/leaderboard/leaderboardCache';
import { clearGroupPhotoCache } from '../pages/groups/groupPhotoCache';
import { clearBlockVisibilityHints } from '../utils/pwaSessionCache';
import { BLOCK_VISIBILITY_EVENT, BLOCK_VISIBILITY_KEY } from '../utils/blockVisibility';

/** Invalidates server-derived state, including mounted media and chat history.
 * Auth stays mounted; routes remount and fetch authoritative visibility. */
export default function BlockVisibilityBoundary({ children }: { children: ReactNode }) {
    const [generation, setGeneration] = useState(0);
    useEffect(() => {
        const invalidate = () => {
            clearBlockVisibilityHints();
            clearAvatarCache();
            clearGroupPhotoCache();
            clearLeaderboardCache();
            setGeneration((value) => value + 1);
        };
        const onStorage = (event: StorageEvent) => {
            if (event.key === BLOCK_VISIBILITY_KEY) invalidate();
        };
        window.addEventListener(BLOCK_VISIBILITY_EVENT, invalidate);
        window.addEventListener('storage', onStorage);
        return () => {
            window.removeEventListener(BLOCK_VISIBILITY_EVENT, invalidate);
            window.removeEventListener('storage', onStorage);
        };
    }, []);
    return (
        <div key={generation} style={{ display: 'contents' }}>
            {children}
        </div>
    );
}
