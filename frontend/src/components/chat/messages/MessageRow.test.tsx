import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import { buildMessageIndex } from './messageIndex';
import MessageRow from './MessageRow';

const message = (id: string, overrides: Partial<Message> = {}): Message => ({
    id,
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    avatar: 'avatar.png',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

const renderRow = (
    id: string,
    overrides: Partial<Omit<React.ComponentProps<typeof MessageRow>, 'message'>> & { message?: Message } = {},
) =>
    render(
        <MemoryRouter>
            <MessageRow
                message={overrides.message ?? message(id)}
                isMe={false}
                isSystem={false}
                grouped={false}
                showSender
                actionsVisible={false}
                canHover
                messageIndex={buildMessageIndex([overrides.message ?? message(id)])}
                reactionPending={null}
                onTapDown={vi.fn()}
                onRevealActions={vi.fn()}
                onDismissActions={vi.fn()}
                onReply={vi.fn()}
                onReaction={vi.fn()}
                {...overrides}
            />
        </MemoryRouter>,
    );

describe('MessageRow', () => {
    it('links the sender name and avatar to the player profile', () => {
        renderRow('message-1');
        expect(screen.getByRole('link', { name: 'bob' })).toHaveAttribute('href', '/profile/user-2');
        expect(screen.getByRole('link', { name: "View bob's profile" })).toHaveAttribute('href', '/profile/user-2');
    });

    it('hides the sender chrome for grouped messages', () => {
        renderRow('message-1', { showSender: false });
        expect(screen.queryByRole('link', { name: 'bob' })).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: "View bob's profile" })).not.toBeInTheDocument();
    });

    it('marks grouped rows for tighter spacing', () => {
        const { container } = renderRow('message-1', { grouped: true });
        expect(container.querySelector('[data-message-id="message-1"]')).toHaveClass('message-grouped');
    });

    it('does not render sender chrome, actions, or reactions for system messages', () => {
        const { container } = render(
            <MemoryRouter>
                <MessageRow
                    message={message('sys', { kind: 'system', content: 'System update' })}
                    isMe={false}
                    isSystem
                    grouped={false}
                    showSender
                    actionsVisible={false}
                    canHover
                    messageIndex={buildMessageIndex([message('sys', { kind: 'system' })])}
                    reactionPending={null}
                    onTapDown={vi.fn()}
                    onRevealActions={vi.fn()}
                    onDismissActions={vi.fn()}
                    onReply={vi.fn()}
                    onReaction={vi.fn()}
                />
            </MemoryRouter>,
        );
        expect(container.querySelector('.message-username')).toBeNull();
        expect(container.querySelector('.avatar-container')).toBeNull();
        expect(container.querySelector('.message-actions')).toBeNull();
        expect(container.querySelector('.message-reactions')).toBeNull();
        expect(screen.getByText('System update')).toBeInTheDocument();
    });

    it('renders the challenge card for challenge messages on the same outer path', () => {
        renderRow('challenge-1', {
            message: message('challenge-1', { kind: 'challenge', photo_id: 'photo-1', content: '' }),
        });
        expect(screen.getByRole('button', { name: /New challenge/i })).toBeInTheDocument();
    });

    it('reveals actions on keyboard focus and hides them again when dismissed', () => {
        const reveal = vi.fn();
        const dismiss = vi.fn();
        const { container } = render(
            <MemoryRouter>
                <MessageRow
                    message={message('message-1')}
                    isMe={false}
                    isSystem={false}
                    grouped={false}
                    showSender
                    actionsVisible={false}
                    canHover
                    messageIndex={buildMessageIndex([message('message-1')])}
                    reactionPending={null}
                    onTapDown={vi.fn()}
                    onRevealActions={reveal}
                    onDismissActions={dismiss}
                    onReply={vi.fn()}
                    onReaction={vi.fn()}
                />
            </MemoryRouter>,
        );
        const row = container.querySelector('[data-message-id="message-1"]') as HTMLElement;
        fireEvent.focus(row);
        expect(reveal).toHaveBeenCalledExactlyOnceWith('message-1');
        expect(dismiss).not.toHaveBeenCalled();
    });

    it('adds the actions-visible class when the actions are revealed', () => {
        const { container } = renderRow('message-1', { actionsVisible: true });
        expect(container.querySelector('[data-message-id="message-1"]')).toHaveClass('actions-visible');
    });

    it('resolves the reply target through the message index without scanning the list', () => {
        const target = message('target', { username: 'alice', content: 'Original text' });
        const reply = message('reply', { reply_to_id: 'target', content: 'Same here' });
        const { container } = render(
            <MemoryRouter>
                <MessageRow
                    message={reply}
                    isMe={false}
                    isSystem={false}
                    grouped={false}
                    showSender
                    actionsVisible={false}
                    canHover
                    messageIndex={buildMessageIndex([target, reply])}
                    reactionPending={null}
                    onTapDown={vi.fn()}
                    onRevealActions={vi.fn()}
                    onDismissActions={vi.fn()}
                    onReply={vi.fn()}
                    onReaction={vi.fn()}
                />
            </MemoryRouter>,
        );
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(container.querySelector('.reply-context')).toHaveTextContent('Original text');
    });

    it('reports the reply action and reveals it for the replying flow', () => {
        const onReply = vi.fn();
        renderRow('message-1', {
            actionsVisible: true,
            onReply,
        });
        fireEvent.click(screen.getByRole('button', { name: 'Reply to bob' }));
        expect(onReply).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ id: 'message-1', username: 'bob' }));
    });
});
