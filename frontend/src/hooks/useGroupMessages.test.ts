import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../types';
import { saveCachedMessages } from '../utils/pwaSessionCache';
import { useGroupMessages } from './useGroupMessages';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
}));

vi.mock('../api', () => ({
    default: { get: mocks.get, post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

// A controllable WebSocket double. The hook assigns its handlers as plain
// fields, so the test can dispatch open/message/close events in any order to
// reproduce reconnect races deterministically.
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

    fireClose(): void {
        this.readyState = MockWebSocket.CLOSED;
        this.onclose?.();
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

const ids = (messages: Message[]): string[] => messages.map((m) => m.id);

beforeEach(() => {
    localStorage.clear();
    mocks.get.mockReset();
    mocks.post.mockReset();
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('useGroupMessages reconnect sequence', () => {
    it('renders cached messages before the socket refresh completes', async () => {
        saveCachedMessages('user-1', 'group-1', [message('cached', '2026-01-01T00:00:00Z')]);
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get.mockResolvedValue({ data: { items: [message('cached', '2026-01-01T00:00:00Z')] } });

        const { result } = renderHook(() => useGroupMessages('group-1', 'user-1'));

        expect(ids(result.current.messages)).toEqual(['cached']);
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
    });

    it('merges catch-up and live delivery without loss or duplicates', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        // Catch-up returns a and b; live delivery repeats b and adds c.
        mocks.get.mockResolvedValue({
            data: { items: [message('a', '2026-01-01T00:00:00Z'), message('b', '2026-01-02T00:00:00Z')] },
        });

        const { result } = renderHook(() => useGroupMessages('group-1'));

        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];

        // Open the renewed socket first; the hook then runs cursor catch-up.
        await act(async () => {
            socket.fireOpen();
        });
        // While catch-up is resolving, live events arrive: b overlaps catch-up
        // and must be deduplicated; c is live-only and must not be lost.
        act(() => socket.fireMessage(message('b', '2026-01-02T00:00:00Z')));
        act(() => socket.fireMessage(message('c', '2026-01-03T00:00:00Z')));

        await waitFor(() => expect(ids(result.current.messages)).toEqual(['a', 'b', 'c']));
        expect(result.current.connectionStatus).toBe('connected');
    });

    it('handles messages delivered before the socket opens', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        // Catch-up returns the same early message plus a newer one.
        mocks.get.mockResolvedValue({
            data: { items: [message('early', '2026-01-01T00:00:00Z'), message('later', '2026-01-02T00:00:00Z')] },
        });

        const { result } = renderHook(() => useGroupMessages('group-1'));

        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];

        // A message the server pushed before the open handshake completes is
        // received by the already-attached onmessage handler.
        act(() => socket.fireMessage(message('early', '2026-01-01T00:00:00Z')));
        // Opening the socket triggers catch-up, which repeats the early message
        // and adds the later one. The early message must not be lost or doubled.
        await act(async () => {
            socket.fireOpen();
        });

        await waitFor(() => expect(ids(result.current.messages)).toEqual(['early', 'later']));
    });

    it('keeps reaction selection viewer-specific while applying live counts and removals', async () => {
        const initial = message('reacted', '2026-01-01T00:00:00Z');
        initial.reactions = [{ emoji: '👍', count: 1, reacted: false }];
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get.mockResolvedValue({ data: { items: [initial] } });

        const { result } = renderHook(() => useGroupMessages('group-1', 'user-1'));
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => socket.fireOpen());
        await waitFor(() => expect(result.current.messages[0]?.reactions?.[0].count).toBe(1));

        act(() =>
            socket.fireMessage({
                ...initial,
                reactions: [{ emoji: '👍', count: 2, reacted: true }],
                reaction_update: { user_id: 'user-2', emoji: '👍', active: true },
            }),
        );
        expect(result.current.messages[0].reactions).toEqual([{ emoji: '👍', count: 2, reacted: false }]);

        act(() =>
            socket.fireMessage({
                ...initial,
                reactions: [{ emoji: '👍', count: 3, reacted: true }],
                reaction_update: { user_id: 'user-1', emoji: '👍', active: true },
            }),
        );
        expect(result.current.messages[0].reactions).toEqual([{ emoji: '👍', count: 3, reacted: true }]);

        act(() =>
            result.current.updateMessage({
                ...initial,
                reactions: [],
            }),
        );
        expect(result.current.messages[0].reactions).toEqual([]);
    });

    it('applies a live resolved challenge update without losing the viewer action', async () => {
        const challenge = {
            ...message('challenge', '2026-01-01T00:00:00Z'),
            kind: 'challenge' as const,
            photo_id: 'photo-1',
            challenge_status: 'available' as const,
        };
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get.mockResolvedValue({ data: { items: [challenge] } });

        const { result } = renderHook(() => useGroupMessages('group-1', 'user-1'));
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => socket.fireOpen());
        await waitFor(() => expect(result.current.messages[0]?.challenge_status).toBe('available'));

        act(() => socket.fireMessage({ ...challenge, challenge_status: undefined, challenge_resolved: true }));
        expect(result.current.messages[0].challenge_resolved).toBe(true);
        expect(result.current.messages[0].challenge_status).toBe('available');
    });

    it('ignores stale messages from a superseded reconnect generation', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        // First generation catch-up returns a; the renewed generation returns c.
        mocks.get
            .mockResolvedValueOnce({ data: { items: [message('a', '2026-01-01T00:00:00Z')] } })
            .mockResolvedValueOnce({ data: { items: [message('c', '2026-01-03T00:00:00Z')] } });

        const { result } = renderHook(() => useGroupMessages('group-1'));

        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const first = MockWebSocket.instances[0];
        await act(async () => {
            first.fireOpen();
        });
        await waitFor(() => expect(ids(result.current.messages)).toEqual(['a']));

        // The connection drops; the hook schedules a renewed reconnect.
        act(() => first.fireClose());
        expect(result.current.connectionStatus).toBe('offline');

        // After the backoff a renewed socket opens and claims a new generation.
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(2), { timeout: 5000 });
        const renewed = MockWebSocket.instances[1];

        // A late message from the superseded first socket must be ignored so it
        // cannot corrupt the renewed generation's view.
        act(() => first.fireMessage(message('stale', '2026-01-02T00:00:00Z')));

        await act(async () => {
            renewed.fireOpen();
        });

        // The renewed catch-up snapshots the last stable cursor (a) before the
        // reconnect, so it fetches only messages after that cursor.
        await waitFor(() => expect(ids(result.current.messages)).toEqual(['a', 'c']));
        expect(mocks.get).toHaveBeenNthCalledWith(
            2,
            '/group/messages',
            expect.objectContaining({
                params: expect.objectContaining({ group_id: 'group-1', after_id: 'a' }),
            }),
        );
    });

    it('tears down the socket and stops reconnecting on unmount', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get.mockResolvedValue({ data: { items: [] } });

        const { result, unmount } = renderHook(() => useGroupMessages('group-1'));
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => {
            socket.fireOpen();
        });

        unmount();

        expect(socket.close).toHaveBeenCalled();
        // After unmount the handlers are detached, so a late message must not
        // mutate state or throw.
        act(() => socket.fireMessage(message('late', '2026-01-01T00:00:00Z')));
        expect(ids(result.current.messages)).toEqual([]);
    });
});
