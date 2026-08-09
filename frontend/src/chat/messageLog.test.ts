import { describe, expect, it } from 'vitest';
import type { Message, Reaction, ReactionUpdate } from '../types';
import { compareMessages, lastStableCursor, mergeMessages, pruneBeforeAnchor } from './messageLog';

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

function update(userID: string, reaction: ReactionUpdate['reaction'], active: boolean): ReactionUpdate {
    return { user_id: userID, reaction, active };
}

const ids = (messages: Message[]): string[] => messages.map((m) => m.id);

describe('compareMessages', () => {
    it('orders by created_at ascending', () => {
        const a = message('a', '2026-01-01T00:00:00Z');
        const b = message('b', '2026-01-02T00:00:00Z');
        expect(compareMessages(a, b)).toBeLessThan(0);
        expect(compareMessages(b, a)).toBeGreaterThan(0);
    });

    it('breaks created_at ties by id ascending', () => {
        const a = message('a', '2026-01-01T00:00:00Z');
        const b = message('b', '2026-01-01T00:00:00Z');
        expect(compareMessages(a, b)).toBeLessThan(0);
        expect(compareMessages(b, a)).toBeGreaterThan(0);
    });

    it('returns zero for the same message', () => {
        const a = message('a', '2026-01-01T00:00:00Z');
        expect(compareMessages(a, a)).toBe(0);
    });
});

describe('mergeMessages', () => {
    it('returns the current log untouched for an empty incoming batch', () => {
        const current = [message('a', '2026-01-01T00:00:00Z')];
        expect(mergeMessages(current, [])).toBe(current);
    });

    it('merges disjoint messages and sorts by the canonical order', () => {
        const current = [message('a', '2026-01-02T00:00:00Z')];
        const result = mergeMessages(current, [message('b', '2026-01-01T00:00:00Z')]);
        expect(ids(result)).toEqual(['b', 'a']);
    });

    it('deduplicates by id, keeping one entry per id', () => {
        const current = [message('a', '2026-01-01T00:00:00Z')];
        const result = mergeMessages(current, [message('a', '2026-01-01T00:00:00Z')]);
        expect(result).toHaveLength(1);
        expect(ids(result)).toEqual(['a']);
    });

    it('skips incoming messages without an id', () => {
        const current = [message('a', '2026-01-01T00:00:00Z')];
        const noId = { ...message('b', '2026-01-02T00:00:00Z'), id: undefined } as unknown as Message;
        const result = mergeMessages(current, [noId]);
        expect(ids(result)).toEqual(['a']);
    });

    it('replaces the existing entry when the same id arrives newer', () => {
        const current = [message('a', '2026-01-01T00:00:00Z', 'old')];
        const result = mergeMessages(current, [message('a', '2026-01-01T00:00:00Z', 'new')]);
        expect(result[0].content).toBe('new');
    });

    it('keeps the previous reactions when the incoming message omits them', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 1, false)] }];
        const result = mergeMessages(current, [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: undefined }]);
        expect(result[0].reactions).toEqual([reaction('like', 1, false)]);
    });

    it('replaces reactions when the incoming message carries an empty list', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 1, false)] }];
        const result = mergeMessages(current, [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [] }]);
        expect(result[0].reactions).toEqual([]);
    });

    it('applies the viewer reaction delta to the viewer-owned reaction', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 1, false)] }];
        const incoming = {
            ...message('a', '2026-01-01T00:00:00Z'),
            reactions: [reaction('like', 2, true)],
            reaction_update: update('user-1', 'like', true),
        };
        const result = mergeMessages(current, [incoming], 'user-1');
        expect(result[0].reactions).toEqual([reaction('like', 2, true)]);
    });

    it('keeps the previous reacted flag for reactions not owned by the viewer', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 1, false)] }];
        const incoming = {
            ...message('a', '2026-01-01T00:00:00Z'),
            reactions: [reaction('like', 2, true)],
            reaction_update: update('user-9', 'like', true),
        };
        const result = mergeMessages(current, [incoming], 'user-1');
        // The other user's reaction count/liked state is applied; the viewer's
        // own selection (not present) falls back to the previous flag.
        expect(result[0].reactions).toEqual([reaction('like', 2, false)]);
    });

    it('applies a viewer deactivation delta', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 2, true)] }];
        const incoming = {
            ...message('a', '2026-01-01T00:00:00Z'),
            reactions: [reaction('like', 1, false)],
            reaction_update: update('user-1', 'like', false),
        };
        const result = mergeMessages(current, [incoming], 'user-1');
        expect(result[0].reactions).toEqual([reaction('like', 1, false)]);
    });

    it('does not treat a delta without a reactions payload as an erase', () => {
        const current = [{ ...message('a', '2026-01-01T00:00:00Z'), reactions: [reaction('like', 1, false)] }];
        const incoming = {
            ...message('a', '2026-01-01T00:00:00Z'),
            reactions: undefined,
            reaction_update: update('user-1', 'like', true),
        };
        const result = mergeMessages(current, [incoming], 'user-1');
        expect(result[0].reactions).toEqual([reaction('like', 1, false)]);
    });

    it('preserves challenge_status when the incoming message omits it', () => {
        const current = [
            {
                ...message('c', '2026-01-01T00:00:00Z'),
                kind: 'challenge' as const,
                photo_id: 'photo-1',
                challenge_status: 'available' as const,
            },
        ];
        const incoming = {
            ...message('c', '2026-01-01T00:00:00Z'),
            kind: 'challenge' as const,
            photo_id: 'photo-1',
            challenge_status: undefined,
        };
        const result = mergeMessages(current, [incoming]);
        expect(result[0].challenge_status).toBe('available');
    });

    it('keeps challenge_resolved sticky once true', () => {
        const current = [
            {
                ...message('c', '2026-01-01T00:00:00Z'),
                kind: 'challenge' as const,
                photo_id: 'photo-1',
                challenge_status: 'available' as const,
            },
        ];
        const incoming = {
            ...message('c', '2026-01-01T00:00:00Z'),
            kind: 'challenge' as const,
            photo_id: 'photo-1',
            challenge_status: undefined,
            challenge_resolved: true,
        };
        const once = mergeMessages(current, [incoming]);
        expect(once[0].challenge_resolved).toBe(true);
        const again = mergeMessages(once, [{ ...incoming, challenge_resolved: false }]);
        expect(again[0].challenge_resolved).toBe(true);
    });

    it('does not mutate the current or incoming arrays', () => {
        const current = [message('a', '2026-01-01T00:00:00Z')];
        const incoming = [message('b', '2026-01-02T00:00:00Z')];
        const currentCopy = [...current];
        const incomingCopy = [...incoming];
        mergeMessages(current, incoming);
        expect(current).toEqual(currentCopy);
        expect(incoming).toEqual(incomingCopy);
    });
});

describe('lastStableCursor', () => {
    it('returns an empty cursor for an empty log', () => {
        expect(lastStableCursor([])).toBe('');
    });

    it('returns the newest message id of a sorted log', () => {
        const log = [message('a', '2026-01-01T00:00:00Z'), message('b', '2026-01-02T00:00:00Z')];
        expect(lastStableCursor(log)).toBe('b');
    });
});

describe('pruneBeforeAnchor', () => {
    it('clears the log when the anchor is null', () => {
        const log = [message('a', '2026-01-01T00:00:00Z')];
        expect(pruneBeforeAnchor(log, null)).toEqual([]);
    });

    it('keeps messages at or newer than the anchor and drops older ones', () => {
        const log = [message('a', '2026-01-01T00:00:00Z'), message('b', '2026-01-02T00:00:00Z')];
        const anchor = message('b', '2026-01-02T00:00:00Z');
        expect(ids(pruneBeforeAnchor(log, anchor))).toEqual(['b']);
    });

    it('keeps the anchor itself when present in the log', () => {
        const log = [message('a', '2026-01-01T00:00:00Z'), message('b', '2026-01-02T00:00:00Z')];
        const anchor = message('a', '2026-01-01T00:00:00Z');
        expect(ids(pruneBeforeAnchor(log, anchor))).toEqual(['a', 'b']);
    });

    it('drops a stale cached message strictly older than the page', () => {
        const cachedStale = message('stale', '2025-12-31T00:00:00Z');
        const cachedLive = message('live', '2026-01-02T00:00:00Z');
        const pageOldest = message('page-a', '2026-01-01T00:00:00Z');
        expect(ids(pruneBeforeAnchor([cachedStale, cachedLive], pageOldest))).toEqual(['live']);
    });
});
