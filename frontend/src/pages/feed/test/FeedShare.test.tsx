import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import FeedShare from '../FeedShare';

afterEach(() => vi.unstubAllGlobals());

it('opens the native share sheet with the challenge link', async () => {
    const share = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { share });
    render(<FeedShare id="post-1" username="Explorer" />);
    fireEvent.click(screen.getByRole('button', { name: 'Share challenge' }));
    expect(share).toHaveBeenCalledWith({
        title: 'GeoGuessMe challenge',
        text: 'Can you find this place? A challenge by Explorer.',
        url: `${window.location.origin}/feed/post-1`,
    });
    await waitFor(() => expect(screen.getByRole('button')).toBeEnabled());
});

it('copies the link when native sharing is unavailable', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    render(<FeedShare id="post-1" username="Explorer" />);
    fireEvent.click(screen.getByRole('button', { name: 'Share challenge' }));
    expect(await screen.findByRole('status')).toHaveTextContent('Link copied');
    expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/feed/post-1`);
});

it('offers a selectable link when clipboard access is denied', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockRejectedValue(new Error('Denied')) } });
    render(<FeedShare id="post-1" username="Explorer" />);
    fireEvent.click(screen.getByRole('button', { name: 'Share challenge' }));
    const input = await screen.findByRole('textbox', { name: 'Copy this link to share' });
    expect(input).toHaveValue(`${window.location.origin}/feed/post-1`);
    expect(input).toHaveAttribute('readonly');
});

it('treats dismissing the share sheet as cancellation and ignores completion after unmount', async () => {
    const share = vi.fn().mockRejectedValueOnce(new DOMException('Cancelled', 'AbortError'));
    vi.stubGlobal('navigator', { share });
    const view = render(<FeedShare id="post-1" username="Explorer" />);
    fireEvent.click(screen.getByRole('button', { name: 'Share challenge' }));
    await waitFor(() => expect(screen.getByRole('button')).toBeEnabled());
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByRole('button')).toBeEnabled();
    let finish!: () => void;
    share.mockReturnValue(
        new Promise<void>((resolve) => {
            finish = resolve;
        }),
    );
    fireEvent.click(screen.getByRole('button', { name: 'Share challenge' }));
    view.unmount();
    await act(async () => finish());
});
