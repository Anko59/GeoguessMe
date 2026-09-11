import { useCallback, useEffect, useRef, useState } from 'react';
import { getAPIErrorMessage, publicFeedAPI } from '../../api';
import type { PublicChallenge, PublicComment, PublicGuessResult } from '../../types';

// Each request owner cancels on unmount and rejects overlapping submissions.
function useFeedRequest() {
    const [pending, setPending] = useState(false);
    const [error, setError] = useState('');
    const request = useRef<AbortController | null>(null);
    useEffect(() => () => request.current?.abort(), []);
    const run = useCallback(async <T>(action: (signal: AbortSignal) => Promise<T>): Promise<T | undefined> => {
        if (request.current && !request.current.signal.aborted) return undefined;
        const controller = new AbortController();
        request.current = controller;
        setPending(true);
        setError('');
        try {
            const value = await action(controller.signal);
            return controller.signal.aborted ? undefined : value;
        } catch (error) {
            if (!controller.signal.aborted)
                setError(getAPIErrorMessage(error, 'Unable to complete this request. Try again.'));
        } finally {
            if (!controller.signal.aborted) {
                request.current = null;
                setPending(false);
            }
        }
    }, []);
    return { run, pending, error };
}

export function useFeed(singleID?: string) {
    const [items, setItems] = useState<PublicChallenge[]>([]);
    const [cursor, setCursor] = useState('');
    const [loaded, setLoaded] = useState(false);
    const { run, pending, error } = useFeedRequest();
    const load = useCallback(
        async (next = '') => {
            const page = await run(async (signal) =>
                singleID
                    ? { items: [await publicFeedAPI.get(singleID, signal)], next_cursor: '' }
                    : publicFeedAPI.list(next, signal),
            );
            if (!page) return;
            setItems((old) =>
                next ? [...old, ...page.items.filter((item) => !old.some((p) => p.id === item.id))] : page.items,
            );
            setCursor(page.next_cursor);
            setLoaded(true);
        },
        [run, singleID],
    );
    useEffect(() => {
        let active = true;
        queueMicrotask(() => {
            if (active) void load();
        });
        return () => {
            active = false;
        };
    }, [load]);
    const update = (post: Partial<PublicChallenge> & { id: string }) =>
        setItems((old) => old.map((p) => (p.id === post.id ? { ...p, ...post } : p)));
    const remove = (id: string) => setItems((old) => old.filter((p) => p.id !== id));
    return { items, cursor, loaded, pending, error, load, update, remove };
}

export function usePublicComments(id: string) {
    const [items, setItems] = useState<PublicComment[]>([]);
    const [cursor, setCursor] = useState('');
    const [loaded, setLoaded] = useState(false);
    const { run, ...request } = useFeedRequest();
    const load = useCallback(
        async (next = '') => {
            const page = await run((signal) => publicFeedAPI.comments(id, next, signal));
            if (!page) return;
            setItems((old) =>
                next ? [...old, ...page.items.filter((item) => !old.some((c) => c.id === item.id))] : page.items,
            );
            setCursor(page.next_cursor);
            setLoaded(true);
        },
        [id, run],
    );
    useEffect(() => {
        let active = true;
        queueMicrotask(() => {
            if (active) void load();
        });
        return () => {
            active = false;
        };
    }, [load]);
    return {
        ...request,
        items,
        cursor,
        loaded,
        load,
        add: (comment: PublicComment) => setItems((old) => [comment, ...old]),
        remove: (commentID: string) => setItems((old) => old.filter((c) => c.id !== commentID)),
    };
}

export function useFeedActions(id: string) {
    const request = useFeedRequest();
    return {
        ...request,
        publish: (form: FormData) => request.run((signal) => publicFeedAPI.publish(form, signal)),
        remove: () => request.run((signal) => publicFeedAPI.remove(id, signal)),
        react: (liked: boolean) => request.run((signal) => publicFeedAPI.react(id, liked, signal)),
        guess: (point: { lat: number; long: number }) =>
            request.run((signal) => publicFeedAPI.guess(id, point, signal)),
        comment: (content: string) => request.run((signal) => publicFeedAPI.comment(id, content, signal)),
        removeComment: (commentID: string) =>
            request.run((signal) => publicFeedAPI.removeComment(id, commentID, signal)),
    };
}

export function usePublicResult(id: string, enabled: boolean) {
    const [result, setResult] = useState<PublicGuessResult | null>(null);
    const { run, ...request } = useFeedRequest();
    const load = useCallback(async () => {
        const value = await run((signal) => publicFeedAPI.result(id, signal));
        if (value) setResult(value);
    }, [id, run]);
    useEffect(() => {
        let active = true;
        queueMicrotask(() => {
            if (active && enabled) void load();
        });
        return () => {
            active = false;
        };
    }, [enabled, load]);
    return { ...request, result, load };
}
