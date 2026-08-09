import { useCallback, useEffect, useLayoutEffect, useReducer, useRef } from 'react';
import api, { getAPIErrorMessage } from '../api';
import {
    createChatSocketController,
    PAGE_SIZE,
    type ChatConnectionStatus,
    type ChatSocketController,
} from '../chat/chatSocketController';
import { chatStreamReducer, initialChatStreamState } from '../chat/chatStream';
import type { Message, MessagesPage } from '../types';
import { readCachedMessages, saveCachedMessages } from '../utils/pwaSessionCache';

export type ConnectionStatus = ChatConnectionStatus;

export interface UseGroupMessagesResult {
    messages: Message[];
    connectionStatus: ConnectionStatus;
    wsRef: React.RefObject<WebSocket | null>;
    error: string;
    updateChallengeStatus: (photoId: string, status: NonNullable<Message['challenge_status']>) => void;
    updateMessage: (message: Message) => void;
    /** Load the page of messages older than the oldest rendered one. */
    loadOlder: () => Promise<void>;
    /** True while more history exists below the rendered messages. */
    hasMoreOlder: boolean;
    loadingOlder: boolean;
}

/**
 * useGroupMessages owns the lossless reconnect sequence for a group's chat.
 *
 * Responsibilities are split by module:
 * - `chat/messageLog` owns the pure merge/dedup/ordering/cursor/anchor logic.
 * - `chat/chatSocketController` owns the WebSocket lifecycle (connection
 *   phases, backoff, generation invalidation) and delivers events through
 *   callbacks.
 * - `chat/chatStream` owns the reducer that resets the stream on a group or
 *   user switch and drops every action from a superseded session.
 * This hook only wires those modules to React, the PWA message cache, and the
 * older-page REST fetch, and exposes the stable public API to the chat UI.
 */
export function useGroupMessages(groupId: string | undefined, userID?: string): UseGroupMessagesResult {
    const cacheIdentity = groupId && userID ? `${userID}:${groupId}` : '';
    const [state, dispatch] = useReducer(chatStreamReducer, cacheIdentity, (identity) =>
        initialChatStreamState(identity, userID, readCachedMessages(userID, groupId)),
    );
    const wsRef = useRef<WebSocket | null>(null);
    const messagesRef = useRef<Message[]>([]);
    const stableCursorRef = useRef<string>('');
    const stoppedRef = useRef(true);
    const loadingOlderRef = useRef(false);
    const controllerRef = useRef<ChatSocketController | null>(null);

    // Reset the whole stream when the cache identity changes. The dispatch
    // runs in a layout effect so the reset lands before the browser paints
    // the frame for the new group: the previous group's messages never flash,
    // and no state is adjusted during render. The reducer no-ops when the
    // identity is unchanged, so an unrelated re-render resets nothing.
    useLayoutEffect(() => {
        dispatch({
            type: 'reset',
            identity: cacheIdentity,
            viewerID: userID,
            cached: readCachedMessages(userID, groupId),
        });
    }, [cacheIdentity, groupId, userID]);

    // Keep a synchronous snapshot of the merged messages so the reconnect
    // sequence can read the last stable cursor without depending on a fresh
    // render cycle.
    useEffect(() => {
        messagesRef.current = state.messages;
    }, [state.messages]);

    useEffect(() => {
        saveCachedMessages(userID, groupId, state.messages);
    }, [groupId, state.messages, userID]);

    const updateMessage = useCallback(
        (message: Message): void => {
            dispatch({ type: 'merge', identity: cacheIdentity, incoming: [message] });
        },
        [cacheIdentity],
    );

    const updateChallengeStatus = useCallback(
        (photoId: string, status: NonNullable<Message['challenge_status']>): void => {
            dispatch({ type: 'challenge_status', identity: cacheIdentity, photoId, status });
        },
        [cacheIdentity],
    );

    // loadOlder prepends the page of messages before the oldest rendered one,
    // like scrolling up in a messaging app. Only one older-page request can be
    // active at a time (loadingOlderRef). The response is dispatched with the
    // session it was requested for; if the group or viewer changed while the
    // request was in flight, the reducer drops it, so a stale page can never
    // update a newer group or session.
    const loadOlder = useCallback(async (): Promise<void> => {
        if (!groupId || stoppedRef.current || loadingOlderRef.current) return;
        const oldest = messagesRef.current[0];
        if (!oldest) return;
        const identity = cacheIdentity;
        loadingOlderRef.current = true;
        dispatch({ type: 'load_older_start', identity });
        try {
            const response = await api.get<MessagesPage | Message[]>('/group/messages', {
                params: { group_id: groupId, before_id: oldest.id, limit: PAGE_SIZE },
            });
            const payload = response.data;
            const items = Array.isArray(payload) ? payload : (payload.items ?? []);
            dispatch({ type: 'load_older_done', identity, items });
        } catch (requestError: unknown) {
            dispatch({
                type: 'load_older_error',
                identity,
                message: getAPIErrorMessage(requestError, 'Unable to load older messages'),
            });
        } finally {
            loadingOlderRef.current = false;
        }
    }, [cacheIdentity, groupId]);

    // The socket controller is owned in a layout effect so that when the
    // group (or the viewer) changes, the previous controller is stopped
    // synchronously before the browser paints the new session's frame. The
    // effect is keyed by the cache identity, so a re-render with the same
    // identity keeps the live connection.
    useLayoutEffect(() => {
        if (!groupId) return;
        stoppedRef.current = false;
        const identity = cacheIdentity;
        const controller = createChatSocketController({
            groupId,
            fetchTicket: async () => {
                const response = await api.post<{ ticket: string }>('/ws/ticket', undefined, {
                    params: { group_id: groupId },
                });
                return response.data.ticket;
            },
            buildSocketURL: (ticket: string): string => {
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                return `${protocol}//${window.location.host}/api/v1/ws?group_id=${encodeURIComponent(groupId)}&ticket=${encodeURIComponent(ticket)}`;
            },
            fetchPage: async (query) => {
                const response = await api.get<MessagesPage | Message[]>('/group/messages', {
                    params: { group_id: groupId, ...query },
                });
                const payload = response.data;
                const items = Array.isArray(payload) ? payload : payload.items;
                // Snapshot the page's stable_cursor so a later reconnect can
                // anchor its catch-up strictly after the newest fetched
                // message. The legacy after_id message-id bridge is gone; the
                // opaque cursor contract is the only catch-up mechanism.
                if (!Array.isArray(payload) && payload.stable_cursor) {
                    stableCursorRef.current = payload.stable_cursor;
                }
                return {
                    items: items ?? [],
                    nextCursor: !Array.isArray(payload) ? (payload.next_cursor ?? undefined) : undefined,
                };
            },
            getLastStableCursor: () => stableCursorRef.current,
            onPhaseChange: (phase) => {
                if (phase === 'stopped') return;
                dispatch({ type: 'status', identity, status: phase });
            },
            onFirstPage: (items) => dispatch({ type: 'first_page', identity, items }),
            onMessages: (incoming) => dispatch({ type: 'merge', identity, incoming }),
            onError: (message) => dispatch({ type: 'error', identity, message }),
            toErrorMessage: getAPIErrorMessage,
            onSocketChange: (socket) => {
                wsRef.current = socket;
            },
        });
        controllerRef.current = controller;
        controller.start();
        return () => {
            stoppedRef.current = true;
            controller.stop();
            controllerRef.current = null;
        };
    }, [cacheIdentity, groupId, dispatch]);

    return {
        messages: state.messages,
        connectionStatus: state.connectionStatus,
        wsRef,
        error: state.error,
        updateChallengeStatus,
        updateMessage,
        loadOlder,
        hasMoreOlder: state.hasMoreOlder,
        loadingOlder: state.loadingOlder,
    };
}
