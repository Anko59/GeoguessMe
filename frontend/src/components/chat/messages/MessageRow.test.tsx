import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import { buildMessageIndex } from './messageIndex';
import MessageRow from './MessageRow';
import { reactionOptions } from '../reactionOptions';

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

const baseProps = (messageOverride?: Message) =>
    ({
        message: messageOverride ?? message('message-1'),
        isMe: false,
        isSystem: false,
        grouped: false,
        showSender: true,
        actionsOpen: false,
        messageIndex: buildMessageIndex([messageOverride ?? message('message-1')]),
        reactionPending: null,
        onToggleActions: vi.fn(),
        onReply: vi.fn(),
        onReaction: vi.fn(),
    }) satisfies Partial<React.ComponentProps<typeof MessageRow>>;

const renderRow = (overrides: Partial<React.ComponentProps<typeof MessageRow>> & { message?: Message } = {}) => {
    const props = { ...baseProps(overrides.message), ...overrides } as React.ComponentProps<typeof MessageRow>;
    return render(
        <MemoryRouter>
            <MessageRow {...props} reactionOptions={overrides.reactionOptions ?? reactionOptions} />
        </MemoryRouter>,
    );
};

beforeEach(() => {
    Element.prototype.scrollIntoView = vi.fn();
});

describe('MessageRow', () => {
    it('links the sender name and avatar to the player profile', () => {
        renderRow();
        expect(screen.getByRole('link', { name: 'bob' })).toHaveAttribute('href', '/profile/user-2');
        expect(screen.getByRole('link', { name: "View bob's profile" })).toHaveAttribute('href', '/profile/user-2');
    });

    it('hides the sender chrome for grouped messages', () => {
        renderRow({ showSender: false });
        expect(screen.queryByRole('link', { name: 'bob' })).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: "View bob's profile" })).not.toBeInTheDocument();
    });

    it('marks grouped rows for tighter spacing', () => {
        const { container } = renderRow({ grouped: true });
        expect(container.querySelector('[data-message-id="message-1"]')).toHaveClass('message-grouped');
    });

    it('does not render sender chrome, actions, or reactions for system messages', () => {
        const { container } = renderRow({
            message: message('sys', { kind: 'system', content: 'System update' }),
            isSystem: true,
        });
        expect(container.querySelector('.message-username')).toBeNull();
        expect(container.querySelector('.avatar-container')).toBeNull();
        expect(container.querySelector('.message-actions')).toBeNull();
        expect(container.querySelector('.message-reactions')).toBeNull();
        expect(screen.getByText('System update')).toBeInTheDocument();
    });

    it('renders the challenge card for challenge messages on the same outer path', () => {
        renderRow({ message: message('challenge-1', { kind: 'challenge', photo_id: 'photo-1', content: '' }) });
        expect(screen.getByText('New challenge')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Accept challenge' })).toBeInTheDocument();
    });

    it('toggles the actions when the message content itself is clicked', () => {
        const onToggleActions = vi.fn();
        renderRow({ onToggleActions });
        fireEvent.click(screen.getByText('Hello'));
        expect(onToggleActions).toHaveBeenCalledExactlyOnceWith('message-1');
    });

    it('does not toggle the actions when the row outside the message is clicked', () => {
        const onToggleActions = vi.fn();
        const { container } = renderRow({ onToggleActions });
        fireEvent.click(container.querySelector('[data-message-id="message-1"]') as HTMLElement);
        expect(onToggleActions).not.toHaveBeenCalled();
    });

    it('does not toggle the actions from nested interactive elements', () => {
        const onToggleActions = vi.fn();
        renderRow({ onToggleActions });
        fireEvent.click(screen.getByRole('link', { name: 'bob' }));
        expect(onToggleActions).not.toHaveBeenCalled();
    });

    it('does not toggle the actions when text inside the message is selected', () => {
        const onToggleActions = vi.fn();
        const selection = { toString: () => 'selected text' };
        const getSelection = vi.spyOn(window, 'getSelection').mockReturnValue(selection as Selection);
        try {
            renderRow({ onToggleActions });
            fireEvent.click(screen.getByText('Hello'));
            expect(onToggleActions).not.toHaveBeenCalled();
        } finally {
            getSelection.mockRestore();
        }
    });

    it('ignores clicks on the open actions panel itself', () => {
        const onToggleActions = vi.fn();
        const { container } = renderRow({ actionsOpen: true, onToggleActions });
        fireEvent.click(container.querySelector('.message-actions') as HTMLElement);
        expect(onToggleActions).not.toHaveBeenCalled();
    });

    it('toggles the actions with Enter and Space on the focused row', () => {
        const onToggleActions = vi.fn();
        const { container } = renderRow({ onToggleActions });
        const row = container.querySelector('[data-message-id="message-1"]') as HTMLElement;
        fireEvent.keyDown(row, { key: 'Enter' });
        fireEvent.keyDown(row, { key: ' ' });
        expect(onToggleActions).toHaveBeenCalledTimes(2);
        expect(onToggleActions).toHaveBeenCalledWith('message-1');
    });

    it('does not toggle the actions from keys pressed on nested controls', () => {
        const onToggleActions = vi.fn();
        renderRow({ onToggleActions });
        fireEvent.keyDown(screen.getByRole('link', { name: 'bob' }), { key: 'Enter' });
        expect(onToggleActions).not.toHaveBeenCalled();
    });

    it('adds the actions-visible class when the actions are open', () => {
        const { container } = renderRow({ actionsOpen: true });
        expect(container.querySelector('[data-message-id="message-1"]')).toHaveClass('actions-visible');
    });

    it('resolves the reply target through the message index without scanning the list', () => {
        const target = message('target', { username: 'alice', content: 'Original text' });
        const reply = message('reply', { reply_to_id: 'target', content: 'Same here' });
        const { container } = render(
            <MemoryRouter>
                <MessageRow
                    {...baseProps(reply)}
                    messageIndex={buildMessageIndex([target, reply])}
                    reactionOptions={reactionOptions}
                />
            </MemoryRouter>,
        );
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(container.querySelector('.reply-context')).toHaveTextContent('Original text');
    });

    it('reports the reply action and reveals it for the replying flow', () => {
        const onReply = vi.fn();
        renderRow({ actionsOpen: true, onReply });
        fireEvent.click(screen.getByRole('button', { name: 'Reply to bob' }));
        expect(onReply).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ id: 'message-1', username: 'bob' }));
    });

    it('opens the challenge from its action button instead of toggling the panel', () => {
        const onToggleActions = vi.fn();
        const onChallengeMessage = vi.fn();
        const challenge = message('challenge-1', { kind: 'challenge', photo_id: 'photo-1', content: '' });
        renderRow({ message: challenge, onToggleActions, onChallengeMessage });
        fireEvent.click(screen.getByRole('button', { name: 'Accept challenge' }));
        expect(onChallengeMessage).toHaveBeenCalledExactlyOnceWith(challenge);
        expect(onToggleActions).not.toHaveBeenCalled();
    });

    it('toggles the panel from the challenge card chrome around the action button', () => {
        const onToggleActions = vi.fn();
        renderRow({
            message: message('challenge-1', { kind: 'challenge', photo_id: 'photo-1', content: '' }),
            onToggleActions,
        });
        fireEvent.click(screen.getByText('New challenge'));
        expect(onToggleActions).toHaveBeenCalledExactlyOnceWith('challenge-1');
    });
});
