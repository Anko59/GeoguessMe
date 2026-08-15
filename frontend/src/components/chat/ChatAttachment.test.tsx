import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ChatAttachment from './ChatAttachment';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:chat-attachment');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
});

describe('ChatAttachment', () => {
    it('opens the shared photo full screen and closes it', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['image'], { type: 'image/png' }) });
        render(<ChatAttachment mediaID="media-1" mediaType="image/png" />);
        const expand = await screen.findByRole('button', { name: 'View Shared photo full screen' });
        fireEvent.click(expand);
        expect(screen.getByRole('dialog', { name: 'Shared photo full screen' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close full-screen photo' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('closes the full-screen photo with the Escape key', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['image'], { type: 'image/png' }) });
        render(<ChatAttachment mediaID="media-1" mediaType="image/png" />);
        const expand = await screen.findByRole('button', { name: 'View Shared photo full screen' });
        fireEvent.click(expand);
        fireEvent.keyDown(window, { key: 'Escape' });
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('keeps videos inline with native controls', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['video'], { type: 'video/mp4' }) });
        const { container } = render(<ChatAttachment mediaID="media-1" mediaType="video/mp4" />);
        await waitFor(() => expect(container.querySelector('video')).not.toBeNull());
        expect(screen.queryByRole('button', { name: 'View Shared photo full screen' })).not.toBeInTheDocument();
    });

    it('shows an error when the media cannot be loaded', async () => {
        mocks.get.mockRejectedValue(new Error('boom'));
        render(<ChatAttachment mediaID="media-1" mediaType="image/png" />);
        expect(await screen.findByText('Unable to load attachment')).toBeInTheDocument();
    });
});
