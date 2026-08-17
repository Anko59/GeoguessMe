import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import MessageContent from './MessageContent';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../../api', () => ({
    default: { get: mocks.get },
}));

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
    mocks.get.mockReset();
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:attachment');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
});

const renderContent = (overrides: Partial<React.ComponentProps<typeof MessageContent>> = {}) =>
    render(<MessageContent message={message()} isMe={false} isSystem={false} {...overrides} />);

describe('MessageContent', () => {
    it('renders the text caption inside a text bubble', () => {
        renderContent();
        const bubble = screen.getByText('Hello').closest('.message-content') as HTMLElement;
        expect(bubble.className).toContain('text');
    });

    it('renders a system message without a bubble styling collision', () => {
        renderContent({ message: message({ kind: 'system', content: 'System update' }), isSystem: true });
        const bubble = screen.getByText('System update').closest('.message-content') as HTMLElement;
        expect(bubble.className).toContain('system-message');
    });

    it('renders the challenge card for challenge messages and opens it from the action button', () => {
        const onChallengeMessage = vi.fn();
        const challenge = message({ kind: 'challenge', photo_id: 'photo-1', content: '' });
        renderContent({
            message: challenge,
            onChallengeMessage,
        });
        const card = screen.getByText('New challenge').closest('.photo-challenge') as HTMLElement;
        expect(card.className).toContain('photo-challenge');
        const action = screen.getByRole('button', { name: 'Accept challenge' });
        expect(action).toHaveClass('start-challenge-btn');
        action.click();
        expect(onChallengeMessage).toHaveBeenCalledExactlyOnceWith(challenge);
    });

    it('renders the media attachment for media messages', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['image'], { type: 'image/png' }) });
        renderContent({
            message: message({ kind: 'media', media_id: 'media-1', media_type: 'image/png', content: '' }),
        });
        expect(await screen.findByRole('button', { name: 'View Shared photo full screen' })).toBeInTheDocument();
    });

    it('shows the reply context with the target sender and content', () => {
        const target = message({ id: 'target', username: 'alice', content: 'Original text' });
        renderContent({
            message: message({ id: 'reply', reply_to_id: 'target', content: 'Same here' }),
            replyTarget: target,
        });
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(screen.getByText('Original text')).toBeInTheDocument();
    });

    it('shows the challenge send time when replying to a challenge', () => {
        const target = message({
            id: 'target',
            username: 'alice',
            kind: 'challenge',
            photo_id: 'photo-1',
            content: '',
            created_at: '2026-01-01T12:30:00Z',
        });
        renderContent({
            message: message({ id: 'reply', reply_to_id: 'target', content: 'Same here' }),
            replyTarget: target,
        });
        const context = screen.getByText(/Message sent at/);
        expect(context).toHaveTextContent(/12:30/);
    });

    it('degrades to Message unavailable when the reply target is missing', () => {
        renderContent({ message: message({ id: 'reply', reply_to_id: 'deleted' }) });
        expect(screen.getByText('Message unavailable')).toBeInTheDocument();
        expect(screen.getByText('Original message')).toBeInTheDocument();
    });
});
