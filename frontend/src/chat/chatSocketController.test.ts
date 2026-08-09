import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../types';
import {
    BASE_RECONNECT_DELAY_MS,
    CATCHUP_LIMIT,
    JITTER_CEILING_MS,
    MAX_RECONNECT_DELAY_MS,
    PAGE_SIZE,
    createChatSocketController,
    reconnectPlan,
    type ChatSocketController,
    type ChatSocketControllerOptions,
    type ChatSocketFetchQuery,
} from './chatSocketController';

// A controllable WebSocket double; tests dispatch open/message/close/error
// events in any order to reproduce reconnect races deterministically.
type SocketHandler = (() => void) | null;
type MessageHandler = ((event: { data: string }) => void) | null;

class MockWebSocket {
    static OPEN = 1 as const;
    static CONNECTING = 0 as const;
    static CLOSING = 2 as const;
    static CLOSED = 3 as const;
    static instances: MockWebSocket[] = [];

    readonly url: string;
    readyState: number = MockWebSocket.CONNECTING;
    onopen: SocketHandler = null;
    onmessage: MessageHandler = null;
    onclose: SocketHandler = null;
    onerror: SocketHandler = null;
    send = vi.fn();
    close = vi.fn(() => {
        this.readyState = MockWebSocket.CLOSED;
    });

    constructor(url: string) {
        this.url = url;
        MockWebSocket.instances.push(this);
    }

    fireOpen(): void {
        this.readyState = MockWebSocket.OPEN;
        this.onopen?.();
    }

    fireMessage(message: Message): void {
        this.onmessage?.({ data: JSON.stringify(message) });
    }

    fireRaw(data: string): void {
        this.onmessage?.({ data });
    }

    fireClose(): void {
        this.readyState = MockWebSocket.CLOSED;
        this.onclose?.();
    }

    fireError(): void {
        this.onerror?.();
    }
}

function message(id: string, createdAt: string, content = id): Message {
    return {
        id,
        group_id: 'group-1',
        user_id: 'user-2',
        username: 'bob',
        avatar: 'avatar.png',
        kind: 'text',
        content,
        created_at: createdAt,
    };
}

interface Harness {
    controller: ChatSocketController;
    sockets: MockWebSocket[];
    delivered: Message[];
    errors: string[];
    phases: string[];
    firstPages: Message[][];
    socketEvents: string[];
    onMessages: ReturnType<typeof vi.fn>;
    onError: ReturnType<typeof vi.fn>;
    onPhaseChange: ReturnType<typeof vi.fn>;
    onFirstPage: ReturnType<typeof vi.fn>;
    onSocketChange: ReturnType<typeof vi.fn>;
    fetchTicket: ReturnType<typeof vi.fn>;
    fetchPage: ReturnType<typeof vi.fn>;
    getLastStableCursor: ReturnType<typeof vi.fn>;
    toErrorMessage: ReturnType<typeof vi.fn>;
}

// Overrides apply on top of defaults; the returned mocks are the exact
// callbacks the controller was built with.
function harness(overrides: Partial<ChatSocketControllerOptions> = {}): Harness {
    const delivered: Message[] = [];
    const errors: string[] = [];
    const phases: string[] = [];
    const firstPages: Message[][] = [];
    const socketEvents: string[] = [];
    const sockets: MockWebSocket[] = [];
    const options: ChatSocketControllerOptions = {
        groupId: 'group-1',
        fetchTicket: vi.fn(async () => 'ticket-1'),
        buildSocketURL: (ticket: string) => `ws://host/api/v1/ws?group_id=group-1&ticket=${ticket}`,
        createSocket: (url: string) => {
            const socket = new MockWebSocket(url);
            sockets.push(socket);
            return socket as unknown as WebSocket;
        },
        fetchPage: vi.fn(async () => ({ items: [] as Message[] })),
        getLastStableCursor: vi.fn(() => ''),
        onPhaseChange: vi.fn((phase: string) => {
            phases.push(phase);
        }),
        onFirstPage: vi.fn((items: Message[]) => {
            firstPages.push(items);
        }),
        onMessages: vi.fn((items: Message[]) => {
            delivered.push(...items);
        }),
        onError: vi.fn((message: string) => {
            errors.push(message);
        }),
        onSocketChange: vi.fn((socket: WebSocket | null) => {
            socketEvents.push(socket ? 'socket:set' : 'socket:null');
        }),
        // The controller only contributes the fallback string; formatting the
        // caught error is the host's job (getAPIErrorMessage).
        toErrorMessage: vi.fn((_error: unknown, fallback: string) => fallback),
        ...overrides,
    };
    const controller = createChatSocketController(options);
    return {
        controller,
        sockets,
        delivered,
        errors,
        phases,
        firstPages,
        socketEvents,
        onMessages: options.onMessages as ReturnType<typeof vi.fn>,
        onError: options.onError as ReturnType<typeof vi.fn>,
        onPhaseChange: options.onPhaseChange as ReturnType<typeof vi.fn>,
        onFirstPage: options.onFirstPage as ReturnType<typeof vi.fn>,
        onSocketChange: options.onSocketChange as ReturnType<typeof vi.fn>,
        fetchTicket: options.fetchTicket as ReturnType<typeof vi.fn>,
        fetchPage: options.fetchPage as ReturnType<typeof vi.fn>,
        getLastStableCursor: options.getLastStableCursor as ReturnType<typeof vi.fn>,
        toErrorMessage: options.toErrorMessage as ReturnType<typeof vi.fn>,
    };
}

// Microtasks are not faked, so plain promise hops flush async controller work.
const flush = async (): Promise<void> => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
};

const deliveredIds = (delivered: Message[]): string[] => delivered.map((m) => m.id);

beforeEach(() => {
    MockWebSocket.instances = [];
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
});

describe('reconnectPlan', () => {
    it('grows exponentially from the base delay, capped at the maximum', () => {
        vi.spyOn(Math, 'random').mockReturnValue(0.5);
        expect(reconnectPlan(0)).toEqual({ delay: BASE_RECONNECT_DELAY_MS + JITTER_CEILING_MS / 2, retry: 1 });
        expect(reconnectPlan(5)).toEqual({ delay: 16000 + JITTER_CEILING_MS / 2, retry: 6 });
        expect(reconnectPlan(6)).toEqual({ delay: MAX_RECONNECT_DELAY_MS + JITTER_CEILING_MS / 2, retry: 7 });
    });

    it('adds jitter within the ceiling', () => {
        vi.spyOn(Math, 'random').mockReturnValue(0.99);
        const high = reconnectPlan(0);
        expect(high.delay).toBeLessThan(BASE_RECONNECT_DELAY_MS + JITTER_CEILING_MS);
        expect(high.delay).toBeGreaterThanOrEqual(BASE_RECONNECT_DELAY_MS);
    });
});

describe('connect sequence', () => {
    it('fetches a ticket, opens the socket, and runs the first sync on open', async () => {
        const h = harness();
        h.controller.start();
        expect(h.phases).toEqual(['connecting']);
        await flush();
        expect(h.fetchTicket).toHaveBeenCalledTimes(1);
        expect(h.sockets).toHaveLength(1);
        expect(h.sockets[0].url).toBe('ws://host/api/v1/ws?group_id=group-1&ticket=ticket-1');
        expect(h.socketEvents).toEqual(['socket:set']);

        h.sockets[0].fireOpen();
        await flush();
        expect(h.phases).toContain('connected');
        // First sync fetches the latest page (no anchor, no cursor).
        expect(h.fetchPage).toHaveBeenCalledWith({ limit: PAGE_SIZE });
        expect(h.firstPages).toHaveLength(1);
        expect(h.firstPages[0]).toEqual([]);
    });

    it('delivers catch-up pages and live events to the host', async () => {
        const h = harness({
            fetchPage: vi.fn(async () => ({ items: [message('a', '2026-01-01T00:00:00Z')] })),
        });
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        expect(deliveredIds(h.delivered)).toEqual(['a']);

        h.sockets[0].fireMessage(message('b', '2026-01-02T00:00:00Z'));
        expect(deliveredIds(h.delivered)).toEqual(['a', 'b']);
    });

    it('drains next_cursor pages, switching to the catch-up limit after the first', async () => {
        let calls = 0;
        const h = harness({
            fetchPage: vi.fn(async () => {
                calls += 1;
                if (calls === 1) return { items: [message('a', '2026-01-01T00:00:00Z')], nextCursor: 'cur-1' };
                return { items: [message('b', '2026-01-02T00:00:00Z')] };
            }),
        });
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        expect(h.fetchPage).toHaveBeenNthCalledWith(1, { limit: PAGE_SIZE });
        expect(h.fetchPage).toHaveBeenNthCalledWith(2, { cursor: 'cur-1', limit: CATCHUP_LIMIT });
        expect(deliveredIds(h.delivered)).toEqual(['a', 'b']);
    });

    it('surfaces a catch-up page failure and stops paging', async () => {
        const h = harness({
            fetchPage: vi.fn(async () => {
                throw new Error('boom');
            }),
        });
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        expect(h.errors).toEqual(['Unable to load messages']);
        expect(h.fetchPage).toHaveBeenCalledTimes(1);
    });

    it('surfaces a ticket failure, goes offline, and schedules a reconnect', async () => {
        vi.useFakeTimers();
        vi.spyOn(Math, 'random').mockReturnValue(0); // deterministic backoff
        const h = harness({
            fetchTicket: vi.fn(async () => {
                throw new Error('no ticket');
            }),
        });
        h.controller.start();
        await flush();
        expect(h.errors).toEqual(['Unable to open chat connection']);
        expect(h.phases).toContain('offline');
        expect(h.sockets).toHaveLength(0);
        expect(h.fetchTicket).toHaveBeenCalledTimes(1);

        // The first reconnect fires after the base backoff (jitter pinned to 0)
        // and attempts a renewed ticket.
        await vi.advanceTimersByTimeAsync(BASE_RECONNECT_DELAY_MS + 1);
        await flush();
        expect(h.fetchTicket).toHaveBeenCalledTimes(2);
    });

    it('surfaces the invalid-chat-message error for malformed live payloads', async () => {
        const h = harness();
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        h.sockets[0].fireRaw('not-json');
        expect(h.errors).toEqual(['Received an invalid chat message']);
    });

    it('ignores live events that carry no message id', async () => {
        const h = harness();
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        h.sockets[0].fireMessage({ ...message('x', '2026-01-01T00:00:00Z'), id: undefined } as unknown as Message);
        expect(deliveredIds(h.delivered)).toEqual([]);
    });
});

describe('reconnect and generation invalidation', () => {
    it('snapshots the cursor before reconnecting and catches up strictly after it', async () => {
        vi.useFakeTimers();
        const h = harness({
            getLastStableCursor: vi.fn(() => 'cursor-a'),
            fetchPage: vi.fn(async (query: ChatSocketFetchQuery) =>
                query.cursor ? { items: [message('c', '2026-01-03T00:00:00Z')] } : { items: [] },
            ),
        });
        h.controller.start();
        await flush();
        const first = h.sockets[0];
        first.fireOpen();
        await flush();
        expect(h.phases).toContain('connected');
        expect(h.getLastStableCursor).toHaveBeenCalledTimes(1);

        first.fireClose();
        expect(h.phases).toContain('offline');
        expect(h.socketEvents).toContain('socket:null');

        await vi.advanceTimersByTimeAsync(MAX_RECONNECT_DELAY_MS);
        await flush();
        expect(h.sockets).toHaveLength(2);
        const renewed = h.sockets[1];
        renewed.fireOpen();
        await flush();
        // The renewed catch-up anchors after the pre-reconnect snapshot and is
        // not a first sync (no cache prune, no hasMore update).
        expect(h.fetchPage).toHaveBeenCalledWith({ cursor: 'cursor-a', limit: PAGE_SIZE });
        expect(h.firstPages).toHaveLength(1);
        expect(deliveredIds(h.delivered)).toEqual(['c']);
    });

    it('ignores messages from a socket superseded by a renewed connection', async () => {
        vi.useFakeTimers();
        const h = harness();
        h.controller.start();
        await flush();
        const first = h.sockets[0];
        first.fireOpen();
        await flush();

        first.fireClose();
        await vi.advanceTimersByTimeAsync(MAX_RECONNECT_DELAY_MS);
        await flush();
        expect(h.sockets).toHaveLength(2);
        // The renewed connect has claimed a new generation, so the old
        // socket's events must be dropped.
        first.fireMessage(message('stale', '2026-01-01T00:00:00Z'));
        expect(deliveredIds(h.delivered)).not.toContain('stale');

        const renewed = h.sockets[1];
        renewed.fireOpen();
        await flush();
        renewed.fireMessage(message('fresh', '2026-01-02T00:00:00Z'));
        expect(deliveredIds(h.delivered)).toEqual(['fresh']);
    });

    it('ignores an abandoned catch-up resolution after a superseding reconnect', async () => {
        vi.useFakeTimers();
        let resolvePage!: (page: { items: Message[]; nextCursor?: string }) => void;
        const h = harness({
            fetchPage: vi.fn(
                () =>
                    new Promise<{ items: Message[]; nextCursor?: string }>((resolve) => {
                        resolvePage = resolve;
                    }),
            ),
        });
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        // Catch-up is pending (resolvePage held) when the socket drops.
        await flush();
        h.sockets[0].fireClose();
        await vi.advanceTimersByTimeAsync(MAX_RECONNECT_DELAY_MS);
        await flush();
        expect(h.sockets).toHaveLength(2);
        // Releasing the abandoned page must not deliver its items.
        resolvePage({ items: [message('abandoned', '2026-01-01T00:00:00Z')] });
        await flush();
        expect(deliveredIds(h.delivered)).toEqual([]);
    });

    it('forwards overlapping live and catch-up deliveries so the host deduplicates', async () => {
        vi.useFakeTimers();
        let resolvePage!: (page: { items: Message[]; nextCursor?: string }) => void;
        const h = harness({
            fetchPage: vi.fn(
                () =>
                    new Promise<{ items: Message[]; nextCursor?: string }>((resolve) => {
                        resolvePage = resolve;
                    }),
            ),
        });
        h.controller.start();
        await flush();
        h.sockets[0].fireOpen();
        await flush();
        // Live event arrives while catch-up is stalled...
        h.sockets[0].fireMessage(message('x', '2026-01-01T00:00:00Z'));
        // ...and catch-up later repeats the same message. The controller
        // delivers both; mergeMessages in the host is the dedup point.
        resolvePage({ items: [message('x', '2026-01-01T00:00:00Z')] });
        await flush();
        expect(deliveredIds(h.delivered)).toEqual(['x', 'x']);
        expect(h.firstPages).toHaveLength(1);
    });
});

describe('stop', () => {
    it('detaches handlers, closes the socket, and clears the reconnect timer', async () => {
        vi.useFakeTimers();
        const h = harness();
        h.controller.start();
        await flush();
        const socket = h.sockets[0];
        socket.fireOpen();
        await flush();
        expect(h.phases).toContain('connected');

        h.controller.stop();
        expect(socket.close).toHaveBeenCalled();
        expect(h.phases).toContain('stopped');
        expect(h.controller.getPhase()).toBe('stopped');

        // Late events on the detached socket must be ignored.
        socket.fireMessage(message('late', '2026-01-01T00:00:00Z'));
        expect(deliveredIds(h.delivered)).not.toContain('late');

        // No reconnect may be scheduled after stop.
        await vi.advanceTimersByTimeAsync(MAX_RECONNECT_DELAY_MS);
        expect(h.sockets).toHaveLength(1);
    });

    it('does not double-close a socket that already closed', async () => {
        vi.useFakeTimers();
        const h = harness();
        h.controller.start();
        await flush();
        const socket = h.sockets[0];
        socket.fireOpen();
        await flush();
        socket.fireClose();
        expect(socket.close).not.toHaveBeenCalled();

        // Stopping after the socket already closed must not call close again
        // (the controller already dropped its reference) and must still stop
        // the phase machine.
        h.controller.stop();
        expect(socket.close).not.toHaveBeenCalled();
        expect(h.controller.getPhase()).toBe('stopped');
    });

    it('clears a pending reconnect timer when stopped between attempts', async () => {
        vi.useFakeTimers();
        const h = harness({
            fetchTicket: vi.fn(async () => {
                throw new Error('no ticket');
            }),
        });
        h.controller.start();
        await flush();
        expect(h.phases).toContain('offline');
        h.controller.stop();
        await vi.advanceTimersByTimeAsync(MAX_RECONNECT_DELAY_MS);
        await flush();
        expect(h.fetchTicket).toHaveBeenCalledTimes(1);
    });
});

describe('phase model', () => {
    it('exposes the current phase and generation', async () => {
        const h = harness();
        expect(h.controller.getPhase()).toBe('stopped');
        expect(h.controller.getGeneration()).toBe(0);
        h.controller.start();
        expect(h.controller.getPhase()).toBe('connecting');
        expect(h.controller.getGeneration()).toBe(1);
        await flush();
        h.sockets[0].fireOpen();
        expect(h.controller.getPhase()).toBe('connected');
        h.controller.stop();
        expect(h.controller.getPhase()).toBe('stopped');
        expect(h.controller.getGeneration()).toBe(2);
    });

    it('never mutates the host through a started-then-stopped controller', async () => {
        const h = harness();
        h.controller.start();
        await flush();
        h.controller.stop();
        expect(h.onSocketChange).toHaveBeenLastCalledWith(null);
        expect(h.delivered).toEqual([]);
        expect(h.errors).toEqual([]);
    });
});
