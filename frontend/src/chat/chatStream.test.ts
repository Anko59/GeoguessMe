import { describe, expect, it } from 'vitest';
import type { Message, Reaction } from '../types';
import { chatStreamReducer, initialChatStreamState, type ChatStreamAction, type ChatStreamState } from './chatStream';

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

function reaction(reaction: Reaction['reaction'], count: number, reacted: boolean): Reaction {
    return { reaction, count, reacted };
}

const ids = (state: ChatStreamState): string[] => state.messages.map((m) => m.id);

function initialState(identity = 'user-1:group-1'): ChatStreamState {
    return initialChatStreamState(identity, 'user-1', []);
}

const act = (state: ChatStreamState, action: ChatStreamAction): ChatStreamState => chatStreamReducer(state, action);

describe('initialChatStreamState', () => {
    it('starts connected-as-pending with the cached messages and empty flags', () => {
        const cached = [message('cached', '2026-01-01T00:00:00Z')];
        const state = initialChatStreamState('user-1:group-1', 'user-1', cached);
        expect(state.identity).toBe('user-1:group-1');
        expect(state.viewerID).toBe('user-1');
        expect(state.messages).toEqual(cached);
        expect(state.connectionStatus).toBe('connecting');
        expect(state.error).toBe('');
        expect(state.loadingOlder).toBe(false);
        expect(state.hasMoreOlder).toBe(false);
    });
});

describe('reset transition', () => {
    it('replaces the whole stream when the identity changes', () => {
        const state = act(initialState(), { type: 'status', identity: 'user-1:group-1', status: 'connected' });
        const cached = [message('other', '2026-01-05T00:00:00Z')];
        const next = act(state, { type: 'reset', identity: 'user-1:group-2', viewerID: 'user-1', cached });
        expect(next.identity).toBe('user-1:group-2');
        expect(ids(next)).toEqual(['other']);
        expect(next.connectionStatus).toBe('connecting');
        expect(next.error).toBe('');
        expect(next.loadingOlder).toBe(false);
        expect(next.hasMoreOlder).toBe(false);
    });

    it('is a no-op for the already-active identity', () => {
        const state = act(initialState(), { type: 'status', identity: 'user-1:group-1', status: 'connected' });
        const next = act(state, { type: 'reset', identity: 'user-1:group-1', viewerID: 'user-1', cached: [] });
        expect(next).toBe(state);
        expect(next.connectionStatus).toBe('connected');
    });
});

describe('session guard', () => {
    it('drops every action from a superseded session', () => {
        const state = act(initialState(), {
            type: 'reset',
            identity: 'user-1:group-2',
            viewerID: 'user-1',
            cached: [],
        });
        const staleMerge = act(state, {
            type: 'merge',
            identity: 'user-1:group-1',
            incoming: [message('stale', '2026-01-01T00:00:00Z')],
        });
        expect(staleMerge).toBe(state);
        const staleStatus = act(state, { type: 'status', identity: 'user-1:group-1', status: 'connected' });
        expect(staleStatus.connectionStatus).toBe('connecting');
        const staleOlder = act(state, {
            type: 'load_older_done',
            identity: 'user-1:group-1',
            items: [message('stale', '2026-01-01T00:00:00Z')],
        });
        expect(ids(staleOlder)).toEqual([]);
        const staleError = act(state, { type: 'error', identity: 'user-1:group-1', message: 'nope' });
        expect(staleError.error).toBe('');
    });
});

describe('live stream transitions', () => {
    it('updates the connection status and error for the active session', () => {
        const connected = act(initialState(), { type: 'status', identity: 'user-1:group-1', status: 'connected' });
        expect(connected.connectionStatus).toBe('connected');
        const failed = act(connected, {
            type: 'error',
            identity: 'user-1:group-1',
            message: 'Unable to load messages',
        });
        expect(failed.error).toBe('Unable to load messages');
    });

    it('merges incoming messages with the canonical order and dedup', () => {
        let state = act(initialState(), {
            type: 'merge',
            identity: 'user-1:group-1',
            incoming: [message('a', '2026-01-02T00:00:00Z')],
        });
        state = act(state, {
            type: 'merge',
            identity: 'user-1:group-1',
            incoming: [message('b', '2026-01-01T00:00:00Z'), message('a', '2026-01-02T00:00:00Z')],
        });
        expect(ids(state)).toEqual(['b', 'a']);
    });

    it('reconciles the viewer reaction delta using the state viewer id', () => {
        const state = act(initialState(), {
            type: 'merge',
            identity: 'user-1:group-1',
            incoming: [
                {
                    ...message('x', '2026-01-01T00:00:00Z'),
                    reactions: [reaction('like', 2, true)],
                    reaction_update: { user_id: 'user-1', reaction: 'like', active: true },
                },
            ],
        });
        expect(state.messages[0].reactions).toEqual([reaction('like', 2, true)]);
    });

    it('prunes cached history against the first page and sets hasMoreOlder', () => {
        const cached = [message('stale', '2025-12-31T00:00:00Z'), message('live', '2026-01-02T00:00:00Z')];
        let state = act(initialChatStreamState('user-1:group-1', 'user-1', cached), {
            type: 'first_page',
            identity: 'user-1:group-1',
            items: [message('page-a', '2026-01-01T00:00:00Z')],
        });
        expect(ids(state)).toEqual(['live']);
        expect(state.hasMoreOlder).toBe(false);

        const fullPage = Array.from({ length: 50 }, (_, i) =>
            message(`p-${i}`, new Date(Date.UTC(2026, 0, 1, 0, i)).toISOString()),
        );
        state = act(initialChatStreamState('user-1:group-1', 'user-1', []), {
            type: 'first_page',
            identity: 'user-1:group-1',
            items: fullPage,
        });
        expect(state.hasMoreOlder).toBe(true);
    });

    it('updates the challenge status of exactly the matching challenge message', () => {
        const challenge = {
            ...message('c', '2026-01-01T00:00:00Z'),
            kind: 'challenge' as const,
            photo_id: 'photo-1',
            challenge_status: 'available' as const,
        };
        const text = message('t', '2026-01-02T00:00:00Z');
        let state = act(initialChatStreamState('user-1:group-1', 'user-1', [challenge, text]), {
            type: 'merge',
            identity: 'user-1:group-1',
            incoming: [],
        });
        state = act(state, {
            type: 'challenge_status',
            identity: 'user-1:group-1',
            photoId: 'photo-1',
            status: 'accepted',
        });
        const updated = state.messages.find((m) => m.kind === 'challenge');
        expect(updated?.challenge_status).toBe('accepted');
        const untouched = state.messages.find((m) => m.kind === 'text');
        expect(untouched?.content).toBe('t');
    });
});

describe('older-page transitions', () => {
    it('flips loadingOlder on start and off on done, merging the page', () => {
        let state = act(initialState(), { type: 'load_older_start', identity: 'user-1:group-1' });
        expect(state.loadingOlder).toBe(true);
        state = act(state, {
            type: 'load_older_done',
            identity: 'user-1:group-1',
            items: [message('a', '2026-01-01T00:00:00Z')],
        });
        expect(state.loadingOlder).toBe(false);
        expect(ids(state)).toEqual(['a']);
        expect(state.hasMoreOlder).toBe(false);
    });

    it('clears loadingOlder on error and surfaces the message', () => {
        let state = act(initialState(), { type: 'load_older_start', identity: 'user-1:group-1' });
        state = act(state, {
            type: 'load_older_error',
            identity: 'user-1:group-1',
            message: 'Unable to load older messages',
        });
        expect(state.loadingOlder).toBe(false);
        expect(state.error).toBe('Unable to load older messages');
    });

    it('flags hasMoreOlder when a full older page returns', () => {
        const page = Array.from({ length: 50 }, (_, i) =>
            message(`m-${i}`, new Date(Date.UTC(2025, 11, 31, 0, i)).toISOString()),
        );
        const state = act(initialState(), {
            type: 'load_older_done',
            identity: 'user-1:group-1',
            items: page,
        });
        expect(state.hasMoreOlder).toBe(true);
    });
});
