import { act } from '@testing-library/react';

type MediaRequest = {
    kind: 'public' | 'timed';
    signal?: AbortSignal;
    promise: Promise<Blob>;
    resolve: (blob: Blob) => void;
    settled: boolean;
};

// API promises, not FeedImage, are controlled. No blob can complete outside
// the test's async act scope, including requests created by subsequent effects.
export function createFeedMediaFixture() {
    const requests: MediaRequest[] = [];
    const pending: MediaRequest[] = [];
    return {
        requests,
        get pendingCount() {
            return pending.length;
        },
        defer(kind: MediaRequest['kind'], signal?: AbortSignal) {
            let resolve!: (blob: Blob) => void;
            const promise = new Promise<Blob>((done) => {
                resolve = done;
            });
            const request = { kind, signal, promise, resolve, settled: false };
            requests.push(request);
            pending.push(request);
            return promise;
        },
        async settle() {
            // Even an initially empty queue needs one act scope: an API list or
            // result snapshot may mount an image effect while React flushes it.
            do {
                await act(async () => {
                    const batch = pending.splice(0);
                    for (const request of batch) {
                        request.settled = true;
                        request.resolve(new Blob([`${request.kind}-image`], { type: 'image/jpeg' }));
                    }
                    await Promise.all(batch.map((request) => request.promise));
                });
            } while (pending.length > 0);
        },
    };
}
