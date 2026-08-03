import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import api from '../../api';
import Chat from './Chat';
import type { Message } from '../../types';

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

const renderChat = (props: Partial<React.ComponentProps<typeof Chat>> = {}) =>
    render(
        <MemoryRouter>
            <Chat
                messages={[message()]}
                wsRef={{ current: null }}
                currentUserId="user-1"
                groupID="group-1"
                {...props}
            />
        </MemoryRouter>,
    );

const mockMatchMedia = (matches: boolean) => {
    window.matchMedia = ((query: string) => ({
        matches,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
    })) as typeof window.matchMedia;
};

beforeEach(() => {
    vi.clearAllMocks();
    Element.prototype.scrollIntoView = vi.fn();
    mockMatchMedia(true);
});

describe('Chat', () => {
    it('shows a sender name once for consecutive messages from the same user', () => {
        const { container } = render(
            <MemoryRouter>
                <Chat
                    messages={[
                        message({ id: 'first', content: 'First message' }),
                        message({ id: 'second', content: 'Second message' }),
                        message({
                            id: 'third',
                            user_id: 'user-3',
                            username: 'carol',
                            content: 'A different sender',
                        }),
                    ]}
                    wsRef={{ current: null }}
                    currentUserId="user-1"
                    groupID="group-1"
                />
            </MemoryRouter>,
        );

        expect(Array.from(container.querySelectorAll('.message-username')).map((node) => node.textContent)).toEqual([
            'bob',
            'carol',
        ]);
        expect(container.querySelector('[data-message-id="second"] .avatar-container')).toHaveClass(
            'avatar-placeholder',
        );
    });

    it('links sender names and avatars to the player profile', () => {
        renderChat();
        expect(screen.getByRole('link', { name: 'bob' })).toHaveAttribute('href', '/profile/user-2');
        expect(screen.getByRole('link', { name: "View bob's profile" })).toHaveAttribute('href', '/profile/user-2');
    });

    it('marks a challenge resolved only for the viewer who answered it', () => {
        const { container } = renderChat({
            messages: [
                message({
                    id: 'ch1',
                    kind: 'challenge',
                    photo_id: 'photo-1',
                    content: '',
                    challenge_status: 'available',
                    challenge_resolved: true,
                }),
                message({
                    id: 'ch2',
                    kind: 'challenge',
                    photo_id: 'photo-2',
                    content: '',
                    challenge_status: 'guessed',
                    challenge_resolved: true,
                }),
            ],
        });
        // Someone else answered ch1; the viewer has not, so it stays a new challenge.
        const othersChallenge = container.querySelector('button[data-photo-id="photo-1"]') as HTMLButtonElement;
        const myChallenge = container.querySelector('button[data-photo-id="photo-2"]') as HTMLButtonElement;
        expect(othersChallenge).not.toHaveClass('resolved');
        expect(othersChallenge).toHaveTextContent('New challenge');
        expect(myChallenge).toHaveClass('resolved');
        expect(myChallenge).toHaveTextContent('Resolved challenge');
    });

    it('labels an expired challenge', () => {
        const { container } = renderChat({
            messages: [
                message({
                    id: 'ch3',
                    kind: 'challenge',
                    photo_id: 'photo-3',
                    content: '',
                    challenge_status: 'expired',
                }),
            ],
        });
        expect(container.querySelector('button[data-photo-id="photo-3"]')).toHaveTextContent('Challenge expired');
    });

    it('shows a round countdown of the challenge deadline', () => {
        const { container } = renderChat({
            messages: [
                message({
                    id: 'ch4',
                    kind: 'challenge',
                    photo_id: 'photo-4',
                    content: '',
                    challenge_status: 'available',
                    challenge_expires_at: new Date(Date.now() + 21 * 3600 * 1000).toISOString(),
                    challenge_ttl_seconds: 24 * 3600,
                }),
            ],
        });
        const timer = container.querySelector('.challenge-timer') as HTMLElement;
        expect(timer).toBeInTheDocument();
        expect(timer).toHaveAttribute('aria-label', '21h remaining');
        expect(timer.textContent).toContain('21h');
    });

    it('renders chat states, sends messages, and opens challenges', () => {
        const send = vi.fn();
        const wsRef = { current: { readyState: WebSocket.OPEN, send } } as unknown as React.RefObject<WebSocket | null>;
        const onChallenge = vi.fn();
        render(
            <MemoryRouter>
                <Chat
                    messages={[
                        message(),
                        message({ id: 'system', kind: 'system', content: 'System update' }),
                        message({
                            id: 'challenge',
                            kind: 'challenge',
                            photo_id: 'photo-1',
                            user_id: 'user-1',
                            content: 'Challenge',
                        }),
                    ]}
                    wsRef={wsRef}
                    currentUserId="user-1"
                    groupID="group-1"
                    connectionStatus="connected"
                    onChallengeMessage={onChallenge}
                />
            </MemoryRouter>,
        );
        expect(screen.getByRole('status')).toHaveTextContent('Connected');
        fireEvent.click(screen.getByRole('button', { name: /challenge/i }));
        expect(onChallenge).toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: '  hi  ' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenCalledWith(JSON.stringify({ content: 'hi' }));

        fireEvent.mouseEnter(screen.getByText('Hello').closest('.message-hover-target') as HTMLElement);
        fireEvent.click(screen.getAllByRole('button', { name: 'Reply to bob' })[0]);
        expect(screen.getAllByRole('status')[1]).toHaveTextContent('Replying to bob');
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'same here' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenLastCalledWith(JSON.stringify({ content: 'same here', reply_to_id: 'message-1' }));

        render(
            <MemoryRouter>
                <Chat
                    messages={[]}
                    wsRef={{ current: null }}
                    currentUserId="user-1"
                    groupID="group-1"
                    connectionStatus="offline"
                />
            </MemoryRouter>,
        );
        expect(screen.getByText('Offline — retrying')).toBeInTheDocument();
        expect(screen.getByText('No messages yet')).toBeInTheDocument();
    });

    it('reveals actions only when hovering the message bubble, not the row', () => {
        renderChat({ connectionStatus: 'connected' });
        const reply = screen.getByRole('button', { name: 'Reply to bob' });
        const row = screen.getByText('Hello').closest('[data-message-id]') as HTMLElement;
        const bubble = screen.getByText('Hello').closest('.message-hover-target') as HTMLElement;

        fireEvent.mouseEnter(row);
        expect(reply).toHaveAttribute('tabindex', '-1');

        fireEvent.mouseEnter(bubble);
        expect(reply).toHaveAttribute('tabindex', '0');
    });

    it('requires a one-second long press on touch instead of opening on tap', () => {
        mockMatchMedia(false);
        vi.useFakeTimers();
        try {
            renderChat({ connectionStatus: 'connected' });
            const reply = screen.getByRole('button', { name: 'Reply to bob' });
            const bubble = screen.getByText('Hello').closest('.message-hover-target') as HTMLElement;

            // A tap (synthetic hover plus a quick pointer sequence) must not open actions.
            fireEvent.mouseEnter(bubble);
            fireEvent.pointerDown(bubble, { isPrimary: true, clientX: 10, clientY: 10 });
            fireEvent.pointerUp(bubble);
            expect(reply).toHaveAttribute('tabindex', '-1');

            // A short hold is still too short.
            fireEvent.pointerDown(bubble, { isPrimary: true, clientX: 10, clientY: 10 });
            act(() => {
                vi.advanceTimersByTime(500);
            });
            fireEvent.pointerUp(bubble);
            expect(reply).toHaveAttribute('tabindex', '-1');

            // Holding for a full second opens them.
            fireEvent.pointerDown(bubble, { isPrimary: true, clientX: 10, clientY: 10 });
            act(() => {
                vi.advanceTimersByTime(1000);
            });
            expect(reply).toHaveAttribute('tabindex', '0');

            // Tapping elsewhere dismisses the actions.
            fireEvent.pointerDown(document.body);
            expect(reply).toHaveAttribute('tabindex', '-1');

            // A hardware-keyboard user on the same touch device can still
            // reveal the actions without a hover-capable pointer.
            fireEvent.focus(bubble.closest('[data-message-id]') as HTMLElement);
            expect(reply).toHaveAttribute('tabindex', '0');
        } finally {
            vi.useRealTimers();
        }
    });

    it('reveals message actions and saves reactions', async () => {
        const put = vi.spyOn(api, 'put').mockResolvedValue({
            data: message({ reactions: [{ reaction: 'like', count: 1, reacted: true }] }),
        });
        const onMessageUpdated = vi.fn();
        renderChat({ connectionStatus: 'connected', onMessageUpdated });

        fireEvent.mouseEnter(screen.getByText('Hello').closest('.message-hover-target') as HTMLElement);
        fireEvent.click(screen.getByRole('button', { name: 'React with thumbs up' }));

        await waitFor(() =>
            expect(put).toHaveBeenCalledWith('/group/message-reactions/message-1', { reaction: 'like' }),
        );
        expect(onMessageUpdated).toHaveBeenCalledWith(
            expect.objectContaining({ reactions: [{ reaction: 'like', count: 1, reacted: true }] }),
        );
    });

    it('shows the members who selected each reaction', () => {
        const { container } = render(
            <MemoryRouter>
                <Chat
                    messages={[
                        message({
                            reactions: [{ reaction: 'like', count: 2, reacted: false, usernames: ['alice', 'carol'] }],
                        }),
                    ]}
                    wsRef={{ current: null }}
                    currentUserId="user-1"
                    groupID="group-1"
                />
            </MemoryRouter>,
        );

        const reactionChip = container.querySelector('.reaction-chip') as HTMLButtonElement;
        expect(reactionChip).toHaveAttribute('aria-label', 'thumbs up reaction, 2. Reacted by alice, carol');
        expect(reactionChip).toHaveAttribute('title', 'Reacted by alice, carol');
        expect(container.querySelector('.reaction-chip-tooltip')).toHaveTextContent('alice, carol');
        expect(container.querySelector('.reaction-chip-image')).toHaveAttribute('src', '/reactions/like.png');
    });

    it('reveals actions for a horizontal swipe but ignores vertical scrolling', () => {
        renderChat({ connectionStatus: 'connected' });
        const messageElement = screen.getByText('Hello').closest('[data-message-id]') as HTMLElement;
        const reply = screen.getByRole('button', { name: 'Reply to bob' });

        fireEvent.pointerDown(messageElement, { isPrimary: true, clientX: 10, clientY: 10 });
        fireEvent.pointerMove(messageElement, { isPrimary: true, clientX: 12, clientY: 45 });
        expect(reply).toHaveAttribute('tabindex', '-1');

        fireEvent.pointerDown(messageElement, { isPrimary: true, clientX: 10, clientY: 10 });
        fireEvent.pointerMove(messageElement, { isPrimary: true, clientX: 45, clientY: 12 });
        expect(reply).toHaveAttribute('tabindex', '0');
    });

    it('uploads a selected photo through the authenticated chat-media endpoint', async () => {
        const send = vi.fn();
        const post = vi.spyOn(api, 'post').mockResolvedValue({ data: {} });
        const wsRef = { current: { readyState: WebSocket.OPEN, send } } as unknown as React.RefObject<WebSocket | null>;
        render(
            <MemoryRouter>
                <Chat
                    messages={[]}
                    wsRef={wsRef}
                    currentUserId="user-1"
                    groupID="group-1"
                    connectionStatus="connected"
                />
            </MemoryRouter>,
        );

        const file = new File(['image'], 'shared.png', { type: 'image/png' });
        const picker = screen.getByLabelText('Attach photo or video');
        fireEvent.change(picker, { target: { files: [file] } });
        expect(screen.getByText('shared.png')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));

        await waitFor(() => expect(post).toHaveBeenCalledWith('/group/messages/media', expect.any(FormData)));
        const form = post.mock.calls[0][1] as FormData;
        expect(form.get('group_id')).toBe('group-1');
        expect(form.get('media')).toBe(file);
        expect(send).not.toHaveBeenCalled();
    });
});
