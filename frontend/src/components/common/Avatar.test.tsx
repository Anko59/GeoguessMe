import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Avatar from './Avatar';
import { bustAvatarCache, isCustomAvatar } from './avatarCache';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({ default: { get: mocks.get } }));

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

describe('isCustomAvatar', () => {
    it('treats the custom marker as custom and everything else as default', () => {
        expect(isCustomAvatar('custom')).toBe(true);
        expect(isCustomAvatar('avatar.png')).toBe(false);
        expect(isCustomAvatar(undefined)).toBe(false);
    });
});

describe('Avatar', () => {
    it('renders a default avatar from the static path without a network call', () => {
        mocks.get.mockResolvedValue({ data: new Blob(['x'], { type: 'image/jpeg' }) });
        render(<Avatar userID="user-1" avatar="avatar.png" username="alice" />);
        const img = screen.getByRole('img');
        expect(img).toHaveAttribute('src', '/avatars/avatar.png');
        expect(mocks.get).not.toHaveBeenCalled();
    });

    it('falls back to the default avatar path for an unknown avatar string', () => {
        render(<Avatar userID="user-1" avatar="weird.png" username="alice" />);
        expect(screen.getByRole('img')).toHaveAttribute('src', '/avatars/weird.png');
        expect(mocks.get).not.toHaveBeenCalled();
    });

    it('fetches the custom avatar as a blob and renders the object url', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['x'], { type: 'image/jpeg' }) });
        render(<Avatar userID="user-1" avatar="custom" username="alice" />);
        const img = await screen.findByRole('img');
        expect(mocks.get).toHaveBeenCalledWith('/users/user-1/avatar', { responseType: 'blob' });
        expect(img).toHaveAttribute('src', 'blob:fake-avatar-url');
    });

    it('reuses the cached object url instead of refetching on a second mount', async () => {
        mocks.get.mockResolvedValue({ data: new Blob(['x'], { type: 'image/jpeg' }) });
        const { unmount } = render(<Avatar userID="user-1" avatar="custom" username="alice" />);
        await screen.findByRole('img'); // wait for the first fetch + cache population
        expect(mocks.get).toHaveBeenCalledTimes(1);
        unmount();

        render(<Avatar userID="user-1" avatar="custom" username="alice" />);
        // A remount of the same user must hit the in-memory cache, not the API.
        await waitFor(() => expect(screen.getByRole('img')).toBeInTheDocument());
        expect(mocks.get).toHaveBeenCalledTimes(1);
    });
});
