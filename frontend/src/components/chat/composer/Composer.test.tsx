import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Message } from '../../../types';
import Composer from './Composer';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('../../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) =>
        error instanceof Error && error.message ? error.message : fallback,
}));

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

const openSocket = (send: (data: string) => void) =>
    ({ current: { readyState: WebSocket.OPEN, send } }) as unknown as React.RefObject<WebSocket | null>;

beforeEach(() => {
    vi.clearAllMocks();
    mocks.post.mockReset();
});

describe('Composer', () => {
    it('sends the trimmed text over the socket', () => {
        const send = vi.fn();
        render(
            <Composer
                wsRef={openSocket(send)}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: '  hi  ' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenCalledExactlyOnceWith(JSON.stringify({ content: 'hi' }));
        expect(screen.getByLabelText('Message')).toHaveValue('');
    });

    it('attaches the reply target id when replying to a message', () => {
        const send = vi.fn();
        render(
            <Composer
                wsRef={openSocket(send)}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={message({ id: 'target' })}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'same here' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).toHaveBeenCalledExactlyOnceWith(JSON.stringify({ content: 'same here', reply_to_id: 'target' }));
    });

    it('shows the reply context and cancels it', () => {
        const onCancelReply = vi.fn();
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={message({ username: 'alice' })}
                onCancelReply={onCancelReply}
            />,
        );
        expect(screen.getByRole('status')).toHaveTextContent('Replying to alice');
        fireEvent.click(screen.getByRole('button', { name: 'Cancel reply' }));
        expect(onCancelReply).toHaveBeenCalledOnce();
    });

    it('does not send while the socket is closed', () => {
        const send = vi.fn();
        render(
            <Composer
                wsRef={
                    { current: { readyState: WebSocket.CLOSED, send } } as unknown as React.RefObject<WebSocket | null>
                }
                groupID="group-1"
                connectionStatus="offline"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'hi' } });
        fireEvent.click(screen.getByRole('button', { name: 'Send message' }));
        expect(send).not.toHaveBeenCalled();
    });

    it('disables the input and picker while offline', () => {
        render(
            <Composer
                wsRef={{ current: null }}
                groupID="group-1"
                connectionStatus="offline"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        expect(screen.getByLabelText('Message')).toBeDisabled();
        expect(screen.getByLabelText('Attach photo or video')).toBeDisabled();
        expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
    });

    it('uploads a selected attachment through the authenticated chat-media endpoint', async () => {
        mocks.post.mockResolvedValue({ data: {} });
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        const file = new File(['image'], 'shared.png', { type: 'image/png' });
        fireEvent.change(screen.getByLabelText('Attach photo or video'), { target: { files: [file] } });
        expect(screen.getByText('shared.png')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));
        await waitFor(() => expect(mocks.post).toHaveBeenCalledWith('/group/messages/media', expect.any(FormData)));
        const form = mocks.post.mock.calls[0][1] as FormData;
        expect(form.get('group_id')).toBe('group-1');
        expect(form.get('media')).toBe(file);
    });

    it('attaches content and reply id to the upload form when present', async () => {
        mocks.post.mockResolvedValue({ data: {} });
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={message({ id: 'target' })}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Message'), { target: { value: 'caption' } });
        fireEvent.change(screen.getByLabelText('Attach photo or video'), {
            target: { files: [new File(['video'], 'clip.mp4', { type: 'video/mp4' })] },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));
        await waitFor(() => expect(mocks.post).toHaveBeenCalled());
        const form = mocks.post.mock.calls[0][1] as FormData;
        expect(form.get('content')).toBe('caption');
        expect(form.get('reply_to_id')).toBe('target');
    });

    it('clears the attachment, draft, and reply context after a successful upload', async () => {
        const onCancelReply = vi.fn();
        mocks.post.mockResolvedValue({ data: {} });
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={message({ id: 'target' })}
                onCancelReply={onCancelReply}
            />,
        );
        fireEvent.change(screen.getByLabelText('Attach photo or video'), {
            target: { files: [new File(['image'], 'shared.png', { type: 'image/png' })] },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));
        await waitFor(() => expect(mocks.post).toHaveBeenCalled());
        await waitFor(() => expect(onCancelReply).toHaveBeenCalled());
        expect(screen.queryByText('shared.png')).not.toBeInTheDocument();
    });

    it('surfaces a readable error when the upload fails', async () => {
        mocks.post.mockRejectedValue(new Error('storage unavailable'));
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Attach photo or video'), {
            target: { files: [new File(['image'], 'shared.png', { type: 'image/png' })] },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Send attachment' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('storage unavailable');
    });

    it('allows removing the staged attachment', () => {
        render(
            <Composer
                wsRef={openSocket(vi.fn())}
                groupID="group-1"
                connectionStatus="connected"
                replyingTo={null}
                onCancelReply={vi.fn()}
            />,
        );
        fireEvent.change(screen.getByLabelText('Attach photo or video'), {
            target: { files: [new File(['image'], 'shared.png', { type: 'image/png' })] },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Remove' }));
        expect(screen.queryByText('shared.png')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Send message' })).toBeDisabled();
    });
});
