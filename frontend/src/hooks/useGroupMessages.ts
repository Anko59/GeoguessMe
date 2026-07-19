import { useCallback, useEffect, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../api';
import type { Message, MessagesPage } from '../types';

export type ConnectionStatus = 'connecting' | 'connected' | 'offline';

const MAX_RECONNECT_DELAY_MS = 30000;
const BASE_RECONNECT_DELAY_MS = 500;
const JITTER_CEILING_MS = 500;
const DEFAULT_LIMIT = 500;

export interface UseGroupMessagesResult {
    messages: Message[];
    connectionStatus: ConnectionStatus;
    wsRef: React.RefObject<WebSocket | null>;
    error: string;
    updateChallengeStatus: (photoId: string, status: NonNullable<Message['challenge_status']>) => void;
}

// reconnectPlan derives the next backoff delay and the incremented retry
// counter. Exponential growth is capped and a small jitter is added so many
// clients do not stampede the server in lockstep after a shared outage.
function reconnectPlan(retry: number): { delay: number; retry: number } {
    const base = Math.min(MAX_RECONNECT_DELAY_MS, BASE_RECONNECT_DELAY_MS * 2 ** retry);
    const jitter = Math.floor(Math.random() * JITTER_CEILING_MS);
    return { delay: base + jitter, retry: retry + 1 };
}

// compareMessages orders messages by the stable tuple (created_at, id), the
// same order the backend paginates by, so the displayed list and the cursor
// snapshot always agree on which message is newest.
function compareMessages(a: Message, b: Message): number {
    const byTime = a.created_at.localeCompare(b.created_at);
    if (byTime !== 0) return byTime;
    if (a.id < b.id) return -1;
    return a.id > b.id ? 1 : 0;
}

/**
 * useGroupMessages owns the lossless reconnect sequence for a group's chat:
 *
 * 1. Snapshot the last stable cursor (the newest known message id) BEFORE
 *    opening the renewed socket.
 * 2. Open the renewed socket so the server starts queueing live events for
 *    this connection.
 * 3. On open, perform a cursor catch-up REST fetch (after the snapshot cursor,
 *    or the latest page when none is known yet).
 * 4. Merge catch-up and live events by message id, deduplicating any message
 *    delivered by both paths.
 * 5. Accept live events from the open socket.
 *
 * A monotonically increasing generation guards every async step: when a new
 * reconnect starts (or the component unmounts) the prior generation is
 * superseded, so messages from a stale socket or an abandoned catch-up can
 * never corrupt the live view or schedule a second overlapping reconnect.
 */
export function useGroupMessages(groupId: string | undefined): UseGroupMessagesResult {
    const [messages, setMessages] = useState<Message[]>([]);
    const [connectionStatus, setConnectionStatus] = useState<ConnectionStatus>('connecting');
    const [error, setError] = useState('');
    const [activeGroup, setActiveGroup] = useState<string | undefined>(groupId);
    const wsRef = useRef<WebSocket | null>(null);
    const messagesRef = useRef<Message[]>([]);
    const generationRef = useRef(0);
    const stoppedRef = useRef(true);
    const retryRef = useRef(0);
    const reconnectTimerRef = useRef<number | undefined>(undefined);

    // Reset per-group state when the group changes. Adjusting state during
    // render (rather than in an effect) is the React idiom for resetting state
    // tied to a prop and avoids a flash of the previous group's messages.
    if (groupId !== activeGroup) {
        setActiveGroup(groupId);
        setMessages([]);
        setConnectionStatus('connecting');
        setError('');
    }

    const mergeMessages = useCallback((incoming: Message[]): void => {
        if (incoming.length === 0) return;
        setMessages((current) => {
            const byId = new Map<string, Message>();
            for (const message of current) {
                if (message.id) byId.set(message.id, message);
            }
            for (const message of incoming) {
                if (message.id) byId.set(message.id, message);
            }
            return [...byId.values()].sort(compareMessages);
        });
    }, []);

    const updateChallengeStatus = useCallback(
        (photoId: string, status: NonNullable<Message['challenge_status']>): void => {
            setMessages((current) =>
                current.map((message) =>
                    message.kind === 'challenge' && message.photo_id === photoId
                        ? { ...message, challenge_status: status }
                        : message,
                ),
            );
        },
        [],
    );

    // Keep a synchronous snapshot of the merged messages so the reconnect
    // sequence can read the last stable cursor without depending on a fresh
    // render cycle.
    useEffect(() => {
        messagesRef.current = messages;
    }, [messages]);

    const lastStableCursor = useCallback((): string => {
        const list = messagesRef.current;
        return list.length > 0 ? list[list.length - 1].id : '';
    }, []);

    const loadAfter = useCallback(
        async (cursor: string, generation: number): Promise<void> => {
            if (!groupId || stoppedRef.current || generation !== generationRef.current) return;
            try {
                const response = await api.get<MessagesPage | Message[]>('/group/messages', {
                    params: {
                        group_id: groupId,
                        limit: DEFAULT_LIMIT,
                        ...(cursor ? { after_id: cursor } : {}),
                    },
                });
                if (stoppedRef.current || generation !== generationRef.current) return;
                const payload = response.data;
                const items = Array.isArray(payload) ? payload : payload.items;
                mergeMessages(items ?? []);
            } catch (requestError: unknown) {
                if (stoppedRef.current || generation !== generationRef.current) return;
                setError(getAPIErrorMessage(requestError, 'Unable to load messages'));
            }
        },
        [groupId, mergeMessages],
    );

    useEffect(() => {
        if (!groupId) return;
        stoppedRef.current = false;
        generationRef.current = 0;
        retryRef.current = 0;

        const clearReconnectTimer = (): void => {
            if (reconnectTimerRef.current !== undefined) {
                window.clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = undefined;
            }
        };

        const scheduleReconnect = (): void => {
            const plan = reconnectPlan(retryRef.current);
            retryRef.current = plan.retry;
            reconnectTimerRef.current = window.setTimeout(() => {
                void connect();
            }, plan.delay);
        };

        const connect = async (): Promise<void> => {
            // Claiming this generation supersedes every prior sequence so its
            // stale socket and catch-up cannot affect state.
            const generation = ++generationRef.current;
            if (stoppedRef.current) return;
            // Snapshot BEFORE opening the renewed socket: catch-up and live
            // delivery then bracket exactly the same gap.
            const cursor = lastStableCursor();
            setConnectionStatus('connecting');
            try {
                const ticketResponse = await api.post<{ ticket: string }>('/ws/ticket', undefined, {
                    params: { group_id: groupId },
                });
                if (stoppedRef.current || generation !== generationRef.current) return;
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                const socket = new WebSocket(
                    `${protocol}//${window.location.host}/api/v1/ws?group_id=${encodeURIComponent(groupId)}&ticket=${encodeURIComponent(ticketResponse.data.ticket)}`,
                );
                wsRef.current = socket;
                socket.onopen = () => {
                    if (stoppedRef.current || generation !== generationRef.current) return;
                    retryRef.current = 0;
                    setConnectionStatus('connected');
                    // Catch up only after the socket is registered so messages
                    // created during the REST window are also delivered live
                    // and deduplicated by id.
                    void loadAfter(cursor, generation);
                };
                socket.onmessage = (event: MessageEvent<string>) => {
                    if (stoppedRef.current || generation !== generationRef.current) return;
                    try {
                        const message = JSON.parse(event.data) as Message;
                        if (message.id) mergeMessages([message]);
                    } catch {
                        setError('Received an invalid chat message');
                    }
                };
                socket.onclose = () => {
                    if (stoppedRef.current || generation !== generationRef.current) return;
                    if (wsRef.current === socket) wsRef.current = null;
                    setConnectionStatus('offline');
                    scheduleReconnect();
                };
                socket.onerror = () => {
                    if (stoppedRef.current || generation !== generationRef.current) return;
                    setConnectionStatus('offline');
                };
            } catch (requestError: unknown) {
                if (stoppedRef.current || generation !== generationRef.current) return;
                setError(getAPIErrorMessage(requestError, 'Unable to open chat connection'));
                setConnectionStatus('offline');
                scheduleReconnect();
            }
        };

        void connect();

        return () => {
            stoppedRef.current = true;
            // Supersede any in-flight sequence so its handlers short-circuit.
            generationRef.current += 1;
            clearReconnectTimer();
            const socket = wsRef.current;
            if (socket) {
                // Detach handlers so the close we trigger cannot schedule a
                // reconnect or mutate state after unmount.
                socket.onopen = null;
                socket.onmessage = null;
                socket.onclose = null;
                socket.onerror = null;
                socket.close();
                wsRef.current = null;
            }
        };
    }, [groupId, lastStableCursor, loadAfter, mergeMessages]);

    return { messages, connectionStatus, wsRef, error, updateChallengeStatus };
}
