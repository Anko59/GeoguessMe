import { fireEvent, render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import MessageReactions from './MessageReactions';

const message = (overrides: Partial<Message> = {}): Message => ({
    id: 'message-1',
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

describe('MessageReactions', () => {
    it('renders nothing when the message has no reactions', () => {
        const { container } = render(<MessageReactions message={message()} onToggle={vi.fn()} />);
        expect(container.querySelector('.message-reactions')).toBeNull();
    });

    it('labels a chip with the reaction, count, and the members who reacted', () => {
        const { container } = render(
            <MessageReactions
                message={message({
                    reactions: [{ reaction: 'like', count: 2, reacted: false, usernames: ['alice', 'carol'] }],
                })}
                onToggle={vi.fn()}
            />,
        );
        const chip = container.querySelector('.reaction-chip') as HTMLButtonElement;
        expect(chip).toHaveAttribute('aria-label', 'thumbs up reaction, 2. Reacted by alice, carol');
        expect(chip).toHaveAttribute('title', 'Reacted by alice, carol');
        expect(chip).toHaveAttribute('aria-pressed', 'false');
        expect(container.querySelector('.reaction-chip-tooltip')).toHaveTextContent('alice, carol');
        expect(container.querySelector('.reaction-chip-image')).toHaveAttribute('src', '/reactions/like.png');
    });

    it('marks the viewer-selected chip as pressed and selected', () => {
        const { container } = render(
            <MessageReactions
                message={message({ reactions: [{ reaction: 'love', count: 1, reacted: true }] })}
                onToggle={vi.fn()}
            />,
        );
        const chip = container.querySelector('.reaction-chip') as HTMLButtonElement;
        expect(chip).toHaveAttribute('aria-pressed', 'true');
        expect(chip.className).toContain('selected');
    });

    it('falls back to the raw reaction text for reactions outside the option set', () => {
        const { container } = render(
            <MessageReactions
                message={message({
                    reactions: [
                        {
                            reaction: 'rocket' as NonNullable<Message['reactions']>[number]['reaction'],
                            count: 3,
                            reacted: false,
                        },
                    ],
                })}
                onToggle={vi.fn()}
            />,
        );
        const chip = container.querySelector('.reaction-chip') as HTMLButtonElement;
        expect(chip).toHaveAttribute('aria-label', 'rocket reaction, 3. Reacted by Unknown user');
        expect(container.querySelector('[aria-hidden="true"]')).toHaveTextContent('rocket');
    });

    it('reports the toggled reaction key', () => {
        const onToggle = vi.fn();
        const { container } = render(
            <MessageReactions
                message={message({ reactions: [{ reaction: 'sad', count: 1, reacted: false }] })}
                onToggle={onToggle}
            />,
        );
        fireEvent.click(container.querySelector('.reaction-chip') as HTMLButtonElement);
        expect(onToggle).toHaveBeenCalledExactlyOnceWith('sad');
    });
});
