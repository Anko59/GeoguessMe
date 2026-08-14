import type { Message } from '../types';

// The socket controller owns the lossless reconnect sequence for a group's
// chat independent of React: connection phases, the reconnect backoff, and
// generation invalidation all live here, and the host (the useGroupMessages
// hook) supplies the REST page fetch and the state-mutation callbacks.
//
// Reconnect sequence:
//  1. Snapshot the last stable cursor (the opaque stable_cursor of the most
//     recent REST page) BEFORE opening the renewed socket.
//  2. Open the renewed socket so the server starts queueing live events for
//     this connection.
//  3. On open, perform a cursor catch-up REST fetch (after the snapshot
//     cursor, or the latest page when none is known yet).
//  4. Deliver catch-up and live events to the host, which merges them by
//     message id so a message delivered by both paths renders exactly once.
//
// A monotonically increasing generation guards every async step: when a new
// reconnect starts (or the controller is stopped) the prior generation is
// superseded, so messages from a stale socket or an abandoned catch-up can
// never corrupt the live view or schedule a second overlapping reconnect.

/** The connection status the host exposes to the chat UI. */
export type ChatConnectionStatus = 'connecting' | 'connected' | 'offline';

/** The explicit connection-phase model. The controller starts in the
 *  'stopped' state, transitions to 'connecting' on every attempt (including
 *  reconnects), 'connected' once a socket opens, 'offline' on close/error,
 *  and back to 'stopped' only when stopped deliberately. */
export type ChatSocketPhase = ChatConnectionStatus | 'stopped';

export const MAX_RECONNECT_DELAY_MS = 30000;
export const BASE_RECONNECT_DELAY_MS = 500;
export const JITTER_CEILING_MS = 500;
// The initial page (and each older page) is capped so opening a long
// conversation stays cheap; older history loads on scroll-up. Forward
// catch-up pages after a reconnect use a larger size and loop until the
// cursor drains so nothing created during a disconnect is ever skipped.
export const PAGE_SIZE = 50;
export const CATCHUP_LIMIT = 200;

// reconnectPlan derives the next backoff delay and the incremented retry
// counter. Exponential growth is capped and a small jitter is added so many
// clients do not stampede the server in lockstep after a shared outage.
export function reconnectPlan(retry: number): { delay: number; retry: number } {
    const base = Math.min(MAX_RECONNECT_DELAY_MS, BASE_RECONNECT_DELAY_MS * 2 ** retry);
    const jitter = Math.floor(Math.random() * JITTER_CEILING_MS);
    return { delay: base + jitter, retry: retry + 1 };
}

/** One normalized REST page returned by the host during cursor catch-up. */
export interface ChatSocketPage {
    items: Message[];
    nextCursor?: string;
}

/** The REST cursor query sent when paging forward after a reconnect. */
export interface ChatSocketFetchQuery {
    cursor?: string;
    limit: number;
}

export interface ChatSocketControllerOptions {
    groupId: string;
    /** Fetch a one-time WebSocket ticket for the group. */
    fetchTicket: () => Promise<string>;
    /** Build the WebSocket URL from the ticket. */
    buildSocketURL: (ticket: string) => string;
    /** Create the socket; injectable so tests can substitute a double. */
    createSocket?: (url: string) => WebSocket;
    /** Fetch one REST page during catch-up, returning items + next cursor. */
    fetchPage: (query: ChatSocketFetchQuery) => Promise<ChatSocketPage>;
    /** Snapshot the newest stable message id before a reconnect. */
    getLastStableCursor: () => string;
    /** Connection-phase transition; 'stopped' is emitted only on stop(). */
    onPhaseChange: (phase: ChatSocketPhase) => void;
    /** First page of the initial sync; the host prunes its cache against it. */
    onFirstPage: (items: Message[]) => void;
    /** A catch-up page or a live event to merge into the log. */
    onMessages: (messages: Message[]) => void;
    /** User-facing error message. */
    onError: (message: string) => void;
    /** Return false for terminal connection errors that must not be retried. */
    shouldReconnect?: (error: unknown) => boolean;
    /** Deliver the live socket so the host can expose and send through it. */
    onSocketChange?: (socket: WebSocket | null) => void;
    /** Format a caught error into a message (defaults to a plain toString). */
    toErrorMessage?: (error: unknown, fallback: string) => string;
}

export interface ChatSocketController {
    /** Begin the connect sequence for the group. */
    start(): void;
    /** Tear down the socket, detach handlers, and stop reconnecting. */
    stop(): void;
    /** The current live socket, or null. */
    getSocket(): WebSocket | null;
    /** The current generation; the host guards async work against it. */
    getGeneration(): number;
    /** The current connection phase. */
    getPhase(): ChatSocketPhase;
}

export function createChatSocketController(options: ChatSocketControllerOptions): ChatSocketController {
    let stopped = true;
    let generation = 0;
    let retry = 0;
    let hasConnected = false;
    let socket: WebSocket | null = null;
    let reconnectTimer: number | undefined;
    let phase: ChatSocketPhase = 'stopped';
    const createSocket = options.createSocket ?? ((url: string) => new WebSocket(url));
    const toErrorMessage =
        options.toErrorMessage ??
        ((error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback));
    const shouldReconnect = options.shouldReconnect ?? (() => true);
    const setPhase = (next: ChatSocketPhase): void => {
        if (phase === next) return;
        phase = next;
        options.onPhaseChange(next);
    };

    const clearReconnectTimer = (): void => {
        if (reconnectTimer !== undefined) {
            window.clearTimeout(reconnectTimer);
            reconnectTimer = undefined;
        }
    };

    const scheduleReconnect = (): void => {
        const plan = reconnectPlan(retry);
        retry = plan.retry;
        reconnectTimer = window.setTimeout(() => {
            void connect();
        }, plan.delay);
    };

    // fetchForward pages through the messages REST endpoint. An empty anchor
    // selects the latest page; a non-empty anchor resumes strictly after that
    // opaque cursor. It follows next_cursor until drained, so reconnect
    // catch-up is lossless even after a long disconnect.
    const fetchForward = async (
        anchorCursor: string,
        currentGeneration: number,
        onPage: (items: Message[]) => void,
    ): Promise<void> => {
        let cursor = '';
        let anchor = anchorCursor;
        let limit = PAGE_SIZE;
        for (;;) {
            if (stopped || currentGeneration !== generation) return;
            try {
                const query: ChatSocketFetchQuery = { limit };
                if (cursor !== '') {
                    query.cursor = cursor;
                } else if (anchor !== '') {
                    query.cursor = anchor;
                }
                const result = await options.fetchPage(query);
                if (stopped || currentGeneration !== generation) return;
                onPage(result.items);
                if (!result.nextCursor) return;
                cursor = result.nextCursor;
                anchor = '';
                limit = CATCHUP_LIMIT;
            } catch (requestError: unknown) {
                if (stopped || currentGeneration !== generation) return;
                options.onError(toErrorMessage(requestError, 'Unable to load messages'));
                return;
            }
        }
    };

    const connect = async (): Promise<void> => {
        // Claiming this generation supersedes every prior sequence so its
        // stale socket and catch-up cannot affect the host's state.
        const currentGeneration = ++generation;
        if (stopped) return;
        // Snapshot BEFORE opening the renewed socket: catch-up and live
        // delivery then bracket exactly the same gap.
        const cursor = options.getLastStableCursor();
        setPhase('connecting');
        try {
            const ticket = await options.fetchTicket();
            if (stopped || currentGeneration !== generation) return;
            const ws = createSocket(options.buildSocketURL(ticket));
            socket = ws;
            options.onSocketChange?.(ws);
            ws.onopen = () => {
                if (stopped || currentGeneration !== generation) return;
                retry = 0;
                setPhase('connected');
                const firstSync = !hasConnected;
                hasConnected = true;
                // A local cache makes the first screen immediate, but the
                // first server sync prunes any cached message older than the
                // fetched page so stale session history can never sit below
                // the live tail with a gap. Live messages arriving during the
                // fetch are newer than the page and survive the prune; older
                // pages still load on scroll-up. Later reconnects use the
                // cursor-only path to stay lossless and inexpensive.
                let firstPageHandled = false;
                const onPage = (items: Message[]): void => {
                    if (firstSync && !firstPageHandled) {
                        firstPageHandled = true;
                        options.onFirstPage(items);
                    }
                    options.onMessages(items);
                };
                void fetchForward(firstSync ? '' : cursor, currentGeneration, onPage);
            };
            ws.onmessage = (event: MessageEvent<string>) => {
                if (stopped || currentGeneration !== generation) return;
                try {
                    const message = JSON.parse(event.data) as Message;
                    if (message.id) options.onMessages([message]);
                } catch {
                    options.onError('Received an invalid chat message');
                }
            };
            ws.onclose = () => {
                if (stopped || currentGeneration !== generation) return;
                if (socket === ws) {
                    socket = null;
                    options.onSocketChange?.(null);
                }
                setPhase('offline');
                scheduleReconnect();
            };
            ws.onerror = () => {
                if (stopped || currentGeneration !== generation) return;
                setPhase('offline');
            };
        } catch (requestError: unknown) {
            if (stopped || currentGeneration !== generation) return;
            options.onError(toErrorMessage(requestError, 'Unable to open chat connection'));
            setPhase('offline');
            if (shouldReconnect(requestError)) scheduleReconnect();
        }
    };

    const stop = (): void => {
        stopped = true;
        // Supersede any in-flight sequence so its handlers short-circuit.
        generation += 1;
        clearReconnectTimer();
        const ws = socket;
        if (ws) {
            // Detach handlers so the close we trigger cannot schedule a
            // reconnect or mutate state after the host is gone.
            ws.onopen = null;
            ws.onmessage = null;
            ws.onclose = null;
            ws.onerror = null;
            ws.close();
            socket = null;
            options.onSocketChange?.(null);
        }
        setPhase('stopped');
    };

    const start = (): void => {
        stopped = false;
        generation = 0;
        retry = 0;
        hasConnected = false;
        void connect();
    };

    return {
        start,
        stop,
        getSocket: () => socket,
        getGeneration: () => generation,
        getPhase: () => phase,
    };
}
