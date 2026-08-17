import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import MessageActions from './MessageActions';
import { reactionOptions } from '../reactionOptions';

const message = (overrides: Partial<Message> = {}): Message => ({
    id: 'message-1',
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    avatar: 'avatar.png',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

const actionsProps = (
    overrides: Partial<React.ComponentProps<typeof MessageActions>> = {},
): React.ComponentProps<typeof MessageActions> => ({
    message: message(),
    visible: true,
    reactionPending: null,
    onReply: vi.fn(),
    onReaction: vi.fn(),
    reactionOptions,
    ...overrides,
});

const renderActions = (overrides: Partial<React.ComponentProps<typeof MessageActions>> = {}) =>
    render(<MessageActions {...actionsProps(overrides)} />);

beforeEach(() => {
    // The panel scrolls itself into view when it opens; jsdom has no layout.
    Element.prototype.scrollIntoView = vi.fn();
});

describe('MessageActions', () => {
    it('is a labelled group and scrolls into view once revealed', () => {
        const { rerender } = render(<MessageActions {...actionsProps()} visible={false} />);
        expect(Element.prototype.scrollIntoView).not.toHaveBeenCalled();
        rerender(<MessageActions {...actionsProps()} visible />);
        expect(Element.prototype.scrollIntoView).toHaveBeenCalledExactlyOnceWith({ block: 'nearest' });
        expect(screen.getByRole('group', { name: 'Message actions' })).toBeInTheDocument();
    });
    it('renders the reply button with the sender name and fires onReply', () => {
        const onReply = vi.fn();
        renderActions({ onReply });
        const reply = screen.getByRole('button', { name: 'Reply to bob' });
        fireEvent.click(reply);
        expect(onReply).toHaveBeenCalledOnce();
    });

    it('renders every reaction option with an accessible label and image', () => {
        renderActions();
        expect(screen.getByRole('button', { name: 'React with thumbs up' })).toBeInTheDocument();
        const love = screen.getByRole('button', { name: 'React with love' });
        expect(love.querySelector('img')).toHaveAttribute('src', '/reactions/love.png');
        expect(love).toHaveAttribute('title', 'love');
        expect(love).toHaveAttribute('aria-pressed', 'false');
    });

    it('marks the viewer-selected reaction as pressed and selected', () => {
        renderActions({
            message: message({ reactions: [{ reaction: 'like', count: 1, reacted: true }] }),
        });
        const like = screen.getByRole('button', { name: 'React with thumbs up' });
        expect(like).toHaveAttribute('aria-pressed', 'true');
        expect(like.className).toContain('selected');
    });

    it('disables the in-flight reaction button', () => {
        renderActions({ reactionPending: 'message-1:love' });
        expect(screen.getByRole('button', { name: 'React with love' })).toBeDisabled();
        expect(screen.getByRole('button', { name: 'React with thumbs up' })).toBeEnabled();
    });

    it('reports the chosen reaction through onReaction', () => {
        const onReaction = vi.fn();
        renderActions({ onReaction });
        fireEvent.click(screen.getByRole('button', { name: 'React with surprised' }));
        expect(onReaction).toHaveBeenCalledExactlyOnceWith('wow');
    });

    it('keeps the actions out of the tab order while hidden', () => {
        renderActions({ visible: false });
        expect(screen.getByRole('button', { name: 'Reply to bob' })).toHaveAttribute('tabindex', '-1');
        expect(screen.getByRole('button', { name: 'React with love' })).toHaveAttribute('tabindex', '-1');
    });

    it('keeps the whole panel out of the tab order when hidden', () => {
        renderActions({ visible: false });
        const container = screen.getByLabelText('Message actions');
        const focusable = container.querySelectorAll('button:not([tabindex="-1"])');
        expect(focusable).toHaveLength(0);
    });
});
