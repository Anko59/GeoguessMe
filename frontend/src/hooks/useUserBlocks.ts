import { useCallback, useEffect, useRef, useState } from 'react';
import { getAPIErrorMessage, userBlocksAPI } from '../api';
import type { BlockedUser } from '../types';

export function useUserBlocks(enabled = true) {
    const [items, setItems] = useState<BlockedUser[]>([]);
    const [loading, setLoading] = useState(enabled);
    const [error, setError] = useState('');
    const [pending, setPending] = useState<string | null>(null);
    const controller = useRef<AbortController | null>(null);

    const reload = useCallback(async (signal: AbortSignal) => {
        setLoading(true);
        setError('');
        try {
            const page = await userBlocksAPI.list(signal);
            if (!signal.aborted) setItems(page.items);
        } catch (failure) {
            if (!signal.aborted) setError(getAPIErrorMessage(failure, 'Unable to load blocked users.'));
        } finally {
            if (!signal.aborted) setLoading(false);
        }
    }, []);

    useEffect(() => {
        const lifetime = new AbortController();
        controller.current = lifetime;
        if (enabled) {
            queueMicrotask(() => {
                if (!lifetime.signal.aborted) void reload(lifetime.signal);
            });
        }
        return () => lifetime.abort();
    }, [enabled, reload]);

    const change = async (userID: string, blocked: boolean): Promise<boolean> => {
        const signal = controller.current?.signal;
        if (!signal || signal.aborted || pending) return false;
        setPending(userID);
        setError('');
        try {
            if (blocked) await userBlocksAPI.block(userID, signal);
            else await userBlocksAPI.unblock(userID, signal);
            if (signal.aborted) return false;
            if (!blocked) setItems((current) => current.filter((item) => item.user_id !== userID));
            return true;
        } catch (failure) {
            if (!signal.aborted) setError(getAPIErrorMessage(failure, 'Unable to update this block. Try again.'));
            return false;
        } finally {
            if (!signal.aborted) setPending(null);
        }
    };

    return {
        items,
        loading,
        error,
        pending,
        change,
        retry: () => {
            const signal = controller.current?.signal;
            if (signal && !signal.aborted) void reload(signal);
        },
    };
}
