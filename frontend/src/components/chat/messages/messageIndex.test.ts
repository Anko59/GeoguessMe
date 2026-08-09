import { renderHook } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { Message } from '../../../types';
import { buildMessageIndex, useMessageIndex } from './messageIndex';

const message = (id: string, overrides: Partial<Message> = {}): Message => ({
    id,
    group_id: 'group-1',
    user_id: 'user-1',
    username: 'bob',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

describe('buildMessageIndex', () => {
    it('maps every message id to its message', () => {
        const first = message('a');
        const second = message('b');
        const index = buildMessageIndex([first, second]);
        expect(index.get('a')).toBe(first);
        expect(index.get('b')).toBe(second);
        expect(index.has('missing')).toBe(false);
    });

    it('lets the later duplicate win the last slot, like a live update replacing a page', () => {
        const original = message('a', { content: 'first version' });
        const replacement = message('a', { content: 'second version' });
        const index = buildMessageIndex([original, replacement]);
        expect(index.get('a')).toBe(replacement);
        expect(index.get('a')?.content).toBe('second version');
    });

    it('handles an empty list', () => {
        expect(buildMessageIndex([]).size).toBe(0);
    });
});

describe('useMessageIndex', () => {
    it('keeps the same index across re-renders and rebuilds only when the list changes', () => {
        const list = [message('a')];
        const { result, rerender } = renderHook(({ messages }) => useMessageIndex(messages), {
            initialProps: { messages: list },
        });
        const first = result.current;
        rerender({ messages: list });
        expect(result.current).toBe(first);

        const grown = [message('a'), message('b')];
        rerender({ messages: grown });
        expect(result.current).not.toBe(first);
        expect(result.current.get('b')).toBe(grown[1]);
    });
});
