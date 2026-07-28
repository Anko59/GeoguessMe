import { useEffect } from 'react';

export function useFaceTrackerPreload(): void {
    useEffect(() => {
        void import('./faceTracker')
            .then(({ preloadFaceTracker }) => preloadFaceTracker())
            .catch(() => {
                // Camera startup retries on demand and reports a user-facing error.
            });
    }, []);
}
