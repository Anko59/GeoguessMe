import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PAGE_SIZE } from '../chat/chatSocketController';
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
        // The first sync fetches the latest page; live delivery repeats b and
        // adds c around it.
        mocks.get.mockResolvedValue({
            data: { items: [message('a', '2026-01-01T00:00:00Z'), message('b', '2026-01-02T00:00:00Z')] },
        });

        const { result } = renderHook(() => useGroupMessages('group-1'));

        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];

        // Open the renewed socket first; the hook then runs the first sync.
        await act(async () => {
            socket.fireOpen();
        });
        // While the first sync is resolving, live events arrive: b overlaps the
        // page and must be deduplicated; c is live-only and must not be lost.
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
        initial.reactions = [{ reaction: 'like', count: 1, reacted: false }];
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
                reactions: [{ reaction: 'like', count: 2, reacted: true }],
                reaction_update: { user_id: 'user-2', reaction: 'like', active: true },
            }),
        );
        expect(result.current.messages[0].reactions).toEqual([{ reaction: 'like', count: 2, reacted: false }]);

        act(() =>
            socket.fireMessage({
                ...initial,
                reactions: [{ reaction: 'like', count: 3, reacted: true }],
                reaction_update: { user_id: 'user-1', reaction: 'like', active: true },
            }),
        );
        expect(result.current.messages[0].reactions).toEqual([{ reaction: 'like', count: 3, reacted: true }]);

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
        // First generation catch-up returns a (with its stable_cursor anchor);
        // the renewed generation returns c.
        mocks.get
            .mockResolvedValueOnce({
                data: { items: [message('a', '2026-01-01T00:00:00Z')], stable_cursor: 'cursor-a' },
            })
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

        // The renewed catch-up snapshots the last stable cursor (cursor-a)
        // before the reconnect, so it fetches only messages after that cursor.
        await waitFor(() => expect(ids(result.current.messages)).toEqual(['a', 'c']));
        expect(mocks.get).toHaveBeenNthCalledWith(
            2,
            '/group/messages',
            expect.objectContaining({
                params: expect.objectContaining({ group_id: 'group-1', cursor: 'cursor-a' }),
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

    it('loads the page before the oldest message and stops when history drains', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get
            .mockResolvedValueOnce({
                data: { items: [message('b', '2026-01-02T00:00:00Z'), message('c', '2026-01-03T00:00:00Z')] },
            })
            .mockResolvedValueOnce({ data: { items: [message('a', '2026-01-01T00:00:00Z')] } });

        const { result } = renderHook(() => useGroupMessages('group-1', 'user-1'));
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => socket.fireOpen());
        await waitFor(() => expect(ids(result.current.messages)).toEqual(['b', 'c']));
        expect(result.current.hasMoreOlder).toBe(false); // short page: history drained

        await act(async () => result.current.loadOlder());
        expect(mocks.get).toHaveBeenLastCalledWith(
            '/group/messages',
            expect.objectContaining({
                params: expect.objectContaining({ group_id: 'group-1', before_id: 'b', limit: 50 }),
            }),
        );
        expect(ids(result.current.messages)).toEqual(['a', 'b', 'c']);
        expect(result.current.hasMoreOlder).toBe(false);
        expect(result.current.loadingOlder).toBe(false);
    });

    it('exposes hasMoreOlder on a full page and ignores concurrent loadOlder calls', async () => {
        const page = Array.from({ length: 50 }, (_, i) =>
            message(`m-${i}`, new Date(Date.UTC(2026, 0, 1, 0, i)).toISOString()),
        );
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get.mockResolvedValueOnce({ data: { items: page } });

        const { result } = renderHook(() => useGroupMessages('group-1', 'user-1'));
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => socket.fireOpen());
        await waitFor(() => expect(result.current.messages).toHaveLength(50));
        expect(result.current.hasMoreOlder).toBe(true); // full page: older history exists

        mocks.get.mockResolvedValueOnce({ data: { items: [message('oldest', '2025-12-31T23:59:00Z')] } });
        await act(async () => {
            const first = result.current.loadOlder();
            const second = result.current.loadOlder(); // ignored while in flight
            await Promise.all([first, second]);
        });
        expect(mocks.get).toHaveBeenCalledTimes(2); // initial sync + one loadOlder
        expect(ids(result.current.messages)).toEqual(['oldest', ...page.map((m) => m.id)]);
        expect(result.current.hasMoreOlder).toBe(false);
    });

    it('resets state and reconnects when the group changes, ignoring stale events', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        mocks.get
            .mockResolvedValueOnce({
                data: { items: [message('a', '2026-01-01T00:00:00Z')], stable_cursor: 'cursor-a' },
            })
            .mockResolvedValueOnce({ data: { items: [], stable_cursor: null } })
            .mockResolvedValueOnce({ data: { items: [] } });

        const { result, rerender } = renderHook(({ gid }: { gid: string }) => useGroupMessages(gid, 'user-1'), {
            initialProps: { gid: 'group-1' },
        });
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const first = MockWebSocket.instances[0];
        await act(async () => first.fireOpen());
        await waitFor(() => expect(result.current.connectionStatus).toBe('connected'));

        // Switching groups resets the stream and opens a fresh connection for
        // the new group.
        rerender({ gid: 'group-2' });
        expect(ids(result.current.messages)).toEqual([]);
        expect(result.current.connectionStatus).toBe('connecting');
        expect(first.close).toHaveBeenCalled();
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(2));
        const renewed = MockWebSocket.instances[1];
        expect(mocks.post).toHaveBeenLastCalledWith('/ws/ticket', undefined, {
            params: { group_id: 'group-2' },
        });

        // A stale live event from the old group's socket cannot surface in the
        // new group.
        act(() => first.fireMessage(message('stale', '2026-01-01T00:00:00Z')));
        expect(ids(result.current.messages)).toEqual([]);

        // The renewed socket works normally.
        await act(async () => renewed.fireOpen());
        await waitFor(() => expect(result.current.connectionStatus).toBe('connected'));

        // Group 2's empty anchor page clears group 1's cursor. A later group 2
        // reconnect must start from group 2's own empty anchor rather than
        // skipping messages behind the foreign cursor.
        act(() => renewed.fireClose());
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(3), { timeout: 5000 });
        const groupTwoReconnect = MockWebSocket.instances[2];
        await act(async () => groupTwoReconnect.fireOpen());
        await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(3));
        expect(mocks.get).toHaveBeenNthCalledWith(3, '/group/messages', {
            params: { group_id: 'group-2', limit: PAGE_SIZE },
        });
    });

    it('drops a stale loadOlder response after the group changes', async () => {
        mocks.post.mockResolvedValue({ data: { ticket: 't' } });
        let releaseOlder!: (value: unknown) => void;
        mocks.get
            .mockResolvedValueOnce({ data: { items: [message('b', '2026-01-02T00:00:00Z')] } }) // group-1 sync
            .mockImplementationOnce(() => new Promise((resolve) => (releaseOlder = resolve))); // loadOlder hangs

        const { result, rerender } = renderHook(({ gid }: { gid: string }) => useGroupMessages(gid, 'user-1'), {
            initialProps: { gid: 'group-1' },
        });
        await waitFor(() => expect(MockWebSocket.instances).toHaveLength(1));
        const socket = MockWebSocket.instances[0];
        await act(async () => socket.fireOpen());
        await waitFor(() => expect(ids(result.current.messages)).toEqual(['b']));

        // Start an older-page fetch and switch groups while it is in flight.
        let pending: Promise<void>;
        act(() => {
            pending = result.current.loadOlder();
        });
        await waitFor(() => expect(result.current.loadingOlder).toBe(true));
        rerender({ gid: 'group-2' });

        // Releasing the stale response must not merge into the new group.
        await act(async () => {
            releaseOlder({ data: { items: [message('stale', '2026-01-01T00:00:00Z')] } });
            await pending;
        });
        expect(ids(result.current.messages)).toEqual([]);
        expect(result.current.loadingOlder).toBe(false);
    });
});
