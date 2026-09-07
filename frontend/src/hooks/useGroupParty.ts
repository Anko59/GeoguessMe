import { useCallback, useEffect, useRef, useState } from 'react';
import api from '../api';
import type { PartyStatus } from '../types';

/**
 * useGroupParty owns a group's Party Time state: an initial load per group,
 * event-driven refreshes, and the exact instant the celebration must end.
 *
 * The server publishes `server_time` with every response so the expiry timer
 * is corrected for client clock skew, mirroring the gameplay countdowns.
 * Exactly one pending timeout exists at a time; when it fires, the callback
 * flips the derived state locally (so the border drops on time even offline)
 * and bumps a re-sync counter whose effect confirms against the server. Every
 * state write happens inside an async or timer callback — never synchronously
 * during render or in an effect body.
 */
export function useGroupParty(groupId: string | undefined): {
    /** Latest known party state for the group; null until the first load. */
    status: PartyStatus | null;
    loading: boolean;
    /**
     * Refresh the authoritative party state. Called when a persisted system
     * message arrives over the socket (a member may have started a party)
     * and after start attempts that ended in a conflict.
     */
    refresh: () => void;
} {
    const [status, setStatus] = useState<PartyStatus | null>(null);
    const [loading, setLoading] = useState(false);
    // Bumped by the expiry timer so the confirmation fetch runs from an
    // effect rather than hiding network work inside a timeout.
    const [resyncCounter, setResyncCounter] = useState(0);
    const requestRef = useRef<{ groupId: string } | null>(null);
    const abortRef = useRef<AbortController | null>(null);
    const timerRef = useRef<number | null>(null);
    // The group whose state is currently loaded or loading; lets a group
    // switch clear stale windows inside the fetch callback instead of in an
    // effect body.
    const loadedGroupRef = useRef<string | null>(null);

    const clearTimer = useCallback((): void => {
        if (timerRef.current !== null) {
            window.clearTimeout(timerRef.current);
            timerRef.current = null;
        }
    }, []);

    const fetchStatus = useCallback(
        async (targetGroupId: string): Promise<void> => {
            const identity = { groupId: targetGroupId };
            requestRef.current = identity;
            abortRef.current?.abort();
            const controller = new AbortController();
            abortRef.current = controller;
            if (loadedGroupRef.current !== targetGroupId) setStatus(null);
            setLoading(true);
            try {
                const response = await api.get<PartyStatus>('/group/party', {
                    params: { group_id: targetGroupId },
                    signal: controller.signal,
                });
                if (requestRef.current !== identity) return;
                loadedGroupRef.current = targetGroupId;
                setStatus(response.data);
                armExpiryTimer(
                    clearTimer,
                    timerRef,
                    response.data,
                    () =>
                        // Optimistic local flip first: the border must drop at
                        // the published end even if the network is unavailable.
                        setStatus((current) =>
                            current
                                ? { ...current, active: false, started_at: undefined, ends_at: undefined }
                                : current,
                        ),
                    () => setResyncCounter((count) => count + 1),
                );
            } catch (requestError: unknown) {
                // Stale or aborted requests never clobber newer state, and a
                // failed refresh keeps the last known status (recent truth)
                // instead of blanking a live celebration; timers and later
                // refreshes self-correct it.
                if ((requestError as { code?: string })?.code === 'ERR_CANCELED') return;
                if (requestRef.current !== identity) return;
            } finally {
                if (requestRef.current === identity) setLoading(false);
            }
        },
        [clearTimer],
    );

    const refresh = useCallback((): void => {
        if (groupId) void fetchStatus(groupId);
    }, [fetchStatus, groupId]);

    // Initial load per group. The cleanup aborts any in-flight request for a
    // superseded group and disarms the expiry timer.
    useEffect(() => {
        if (!groupId) return undefined;
        // The repository's fetch-in-effect idiom: schedule through a
        // microtask so no state update runs synchronously in the effect.
        queueMicrotask(() => void fetchStatus(groupId));
        return () => {
            requestRef.current = null;
            abortRef.current?.abort();
            clearTimer();
        };
    }, [clearTimer, fetchStatus, groupId]);

    // Confirmation pass after the expiry timer fired: the optimistic flip
    // already stopped the celebration; this re-syncs authoritative state.
    // A group whose state was never loaded here skips the pass — its regular
    // initial load already covers it.
    useEffect(() => {
        if (resyncCounter === 0 || !groupId || loadedGroupRef.current !== groupId) return;
        queueMicrotask(() => void fetchStatus(groupId));
    }, [fetchStatus, groupId, resyncCounter]);

    return { status, loading, refresh };
}

/**
 * Arm the single expiry timer from a freshly fetched status. The timer fires
 * at the earliest relevant deadline: the active party's end, or — once the
 * party ended and recharging still blocks — the moment the cooldown lifts. A
 * deadline already in the past schedules with zero delay so the same callback
 * path applies the transition.
 */
function armExpiryTimer(
    clearTimer: () => void,
    timerRef: React.MutableRefObject<number | null>,
    next: PartyStatus,
    markExpired: () => void,
    requestResync: () => void,
): void {
    clearTimer();
    const deadlines: number[] = [];
    if (next.ends_at) deadlines.push(Date.parse(next.ends_at));
    if (!next.active && next.next_available_at) deadlines.push(Date.parse(next.next_available_at));
    if (deadlines.length === 0) return;
    const offset = Date.parse(next.server_time) - Date.now();
    const delay = Math.max(0, Math.min(...deadlines) - (Date.now() + offset));
    timerRef.current = window.setTimeout(() => {
        timerRef.current = null;
        markExpired();
        requestResync();
    }, delay);
}
