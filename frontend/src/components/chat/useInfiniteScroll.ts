import { useEffect, useRef } from 'react';

/**
 * Chat-scroll behavior for history pagination.
 *
 * - Requests the older page when the user scrolls to the top (at most once per
 *   load, guarded by the loadingOlder flag).
 * - Keeps the viewport anchored while the older page is prepended, so reading
 *   history is never interrupted by a scroll jump.
 * - Only auto-scrolls to the newest message while the user is already at the
 *   bottom, so a live message never yanks a reader out of older history.
 */
export function useInfiniteScroll(
    scrollContainerRef: React.RefObject<HTMLDivElement | null>,
    endRef: React.RefObject<HTMLDivElement | null>,
    messagesDependency: unknown,
    loadingOlder: boolean,
    hasMoreOlder: boolean,
    onLoadOlder: (() => void) | undefined,
): { onScroll: () => void } {
    const atBottomRef = useRef(true);
    const pendingOlderScrollRef = useRef<number | null>(null);
    const loadOlderRequestedRef = useRef(false);

    useEffect(() => {
        if (!loadingOlder) loadOlderRequestedRef.current = false;
    }, [loadingOlder]);

    useEffect(() => {
        const list = scrollContainerRef.current;
        if (!list) return;
        // After an older page is prepended, restore the previous scroll anchor
        // so the visible conversation does not jump; otherwise keep the view
        // pinned to the bottom only while the user is already there.
        if (pendingOlderScrollRef.current !== null) {
            list.scrollTop = list.scrollHeight - pendingOlderScrollRef.current;
            pendingOlderScrollRef.current = null;
            return;
        }
        if (atBottomRef.current) {
            endRef.current?.scrollIntoView({ behavior: 'smooth' });
        }
    }, [endRef, messagesDependency, scrollContainerRef]);

    const onScroll = (): void => {
        const list = scrollContainerRef.current;
        if (!list) return;
        const nearBottom = list.scrollHeight - list.scrollTop - list.clientHeight < 96;
        atBottomRef.current = nearBottom;
        if (list.scrollTop < 48 && hasMoreOlder && !loadingOlder && !loadOlderRequestedRef.current) {
            loadOlderRequestedRef.current = true;
            pendingOlderScrollRef.current = list.scrollHeight;
            void onLoadOlder?.();
        }
    };

    return { onScroll };
}
