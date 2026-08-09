import { act, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import type { ChallengeResults, Message, User } from '../../types';
import Game from './Game';

const mocks = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }));

vi.mock('../../api', () => ({
    default: { get: mocks.get, post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

vi.mock('../map/Map', () => ({ default: () => <div>Map</div> }));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
};

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

const challenge = (photoId: string): Message => ({
    id: `message-${photoId}`,
    group_id: 'group-1',
    user_id: user.id,
    username: user.username,
    avatar: user.avatar,
    kind: 'challenge',
    photo_id: photoId,
    created_at: '2026-01-01T00:00:00Z',
});

const results = (photoId: string, mediaUrl: string): ChallengeResults => ({
    photo_id: photoId,
    group_id: 'group-1',
    actual_lat: 48,
    actual_long: 2,
    media_available: true,
    media_url: mediaUrl,
    media_type: 'image/jpeg',
    server_time: new Date().toISOString(),
    guesses: [],
});

const tree = (photoId: string) => (
    <AuthContext.Provider value={authValue}>
        <MemoryRouter>
            <Game gameMessage={challenge(photoId)} onClose={vi.fn()} />
        </MemoryRouter>
    </AuthContext.Provider>
);

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    mocks.post.mockReset();
    Element.prototype.scrollIntoView = vi.fn();
});

describe('Game asynchronous ownership', () => {
    it('ignores an older challenge result that resolves after a replacement', async () => {
        let resolveA!: (value: { data: ChallengeResults }) => void;
        mocks.get.mockImplementation((url: string) => {
            if (url.includes('photo-a')) return new Promise((resolve) => (resolveA = resolve));
            return Promise.resolve({ data: results('photo-b', 'https://example.test/photo-b.jpg') });
        });

        const view = render(tree('photo-a'));
        await waitFor(() => expect(resolveA).toBeTypeOf('function'));
        view.rerender(tree('photo-b'));
        const open = await screen.findByRole('button', { name: 'View Challenge location full screen' });
        expect(open.querySelector('img')).toHaveAttribute('src', 'https://example.test/photo-b.jpg');

        await act(async () => {
            resolveA({ data: results('photo-a', 'https://example.test/photo-a.jpg') });
            await Promise.resolve();
        });
        expect(open.querySelector('img')).toHaveAttribute('src', 'https://example.test/photo-b.jpg');
    });

    it('revokes a blob created after the challenge unmounts', async () => {
        let resolveBlob!: (value: { data: Blob }) => void;
        mocks.get
            .mockResolvedValueOnce({ data: results('photo-a', '/api/v1/challenges/photo-a/media') })
            .mockImplementationOnce(() => new Promise((resolve) => (resolveBlob = resolve)));
        const create = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:late-photo');
        const revoke = vi.spyOn(URL, 'revokeObjectURL');

        const view = render(tree('photo-a'));
        await waitFor(() => expect(resolveBlob).toBeTypeOf('function'));
        view.unmount();
        await act(async () => {
            resolveBlob({ data: new Blob(['late'], { type: 'image/jpeg' }) });
            await Promise.resolve();
        });

        expect(create).toHaveBeenCalledTimes(1);
        expect(revoke).toHaveBeenCalledTimes(1);
        expect(revoke).toHaveBeenCalledWith('blob:late-photo');
    });
});
