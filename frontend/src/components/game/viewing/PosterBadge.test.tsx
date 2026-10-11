import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import PosterBadge from './PosterBadge';
import { bustAvatarCache } from '../../common/avatarCache';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../../api', () => ({ default: { get: mocks.get } }));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:fake-avatar-url');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => undefined);
});

afterEach(() => {
    vi.restoreAllMocks();
    bustAvatarCache('user-1');
});

describe('PosterBadge', () => {
    it('identifies the submitting player with their avatar and name', () => {
        const { container } = render(
            <PosterBadge poster={{ userId: 'user-2', username: 'bob', avatar: 'avatar.png' }} />,
        );

        expect(screen.getByText('Posted by')).toBeInTheDocument();
        expect(screen.getByText('bob')).toBeInTheDocument();
        // Default avatars render from the static path with no network call.
        expect(container.querySelector('.challenge-poster__avatar img')).toHaveAttribute('src', '/avatars/avatar.png');
        // The avatar is decorative: the name is already visible text.
        expect(container.querySelector('.challenge-poster__avatar')).toHaveAttribute('aria-hidden', 'true');
        expect(mocks.get).not.toHaveBeenCalled();
    });

    it('fetches a custom avatar once through the shared session cache', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['x'], { type: 'image/jpeg' }) });
        const { container, unmount } = render(
            <PosterBadge poster={{ userId: 'user-1', username: 'alice', avatar: 'custom' }} />,
        );

        expect(mocks.get).toHaveBeenCalledWith('/users/user-1/avatar', { responseType: 'blob' });
        // The placeholder circle renders until the fetched blob URL resolves.
        await waitFor(() =>
            expect(container.querySelector('.challenge-poster__avatar img')).toHaveAttribute(
                'src',
                'blob:fake-avatar-url',
            ),
        );
        unmount();

        // A second badge for the same player reuses the cached object URL.
        const second = render(<PosterBadge poster={{ userId: 'user-1', username: 'alice', avatar: 'custom' }} />);
        expect(second.container.querySelector('.challenge-poster__avatar img')).toHaveAttribute(
            'src',
            'blob:fake-avatar-url',
        );
        expect(mocks.get).toHaveBeenCalledTimes(1);
    });

    it('falls back to the default avatar and player label when the identity is missing', () => {
        const { container } = render(<PosterBadge poster={{ userId: 'user-2' }} />);

        expect(screen.getByText('Posted by')).toBeInTheDocument();
        expect(screen.getByText('Unknown player')).toBeInTheDocument();
        expect(container.querySelector('.challenge-poster__avatar img')).toHaveAttribute('src', '/avatars/avatar.png');
    });
});
