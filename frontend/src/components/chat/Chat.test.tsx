import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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

beforeEach(() => {
    vi.clearAllMocks();
    Element.prototype.scrollIntoView = vi.fn();
});

describe('Chat', () => {
    it('renders chat states, sends messages, and opens challenges', () => {
        const send = vi.fn();
        const wsRef = { current: { readyState: WebSocket.OPEN, send } } as unknown as React.RefObject<WebSocket | null>;
        const onChallenge = vi.fn();
        render(
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
                connectionStatus="connected"
                onChallengeMessage={onChallenge}
            />,
        );
        expect(screen.getByRole('status')).toHaveTextContent('Connected');
        fireEvent.click(screen.getByRole('button', { name: /challenge/i }));
        expect(onChallenge).toHaveBeenCalled();
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: '  hi  ' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenCalledWith(JSON.stringify({ content: 'hi' }));

        render(<Chat messages={[]} wsRef={{ current: null }} currentUserId="user-1" connectionStatus="offline" />);
        expect(screen.getByText('Offline — retrying')).toBeInTheDocument();
        expect(screen.getByText('No messages yet')).toBeInTheDocument();
    });
});
