import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import type { User } from '../../types';
import ProfilePage from './ProfilePage';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../utils/mapPins';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    profileLeaderboard: vi.fn(),
    listBlocks: vi.fn(),
    block: vi.fn(),
    unblock: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
    publicFeedAPI: { profileLeaderboard: mocks.profileLeaderboard },
    userBlocksAPI: { list: mocks.listBlocks, block: mocks.block, unblock: mocks.unblock },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    email_verified_at: null,
    avatar: 'avatar.png',
    password_login_enabled: true,
    oidc_linked: false,
    migration_required: false,
};

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

const profile = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    email_verified_at: null,
    avatar: 'avatar.png',
    total_points: 6000,
    guess_count: 4,
    average_score: 1500,
    elo: 1152,
    rank: {
        level: 2,
        name: 'Lost Tourist',
        min_points: 5000,
        next_points: 15000,
        points_in_rank: 1000,
        points_to_next: 10000,
        progress_percent: 10,
        trophy_key: 'lost-tourist',
        next_rank: {
            level: 3,
            name: 'Clueless Wanderer',
            min_points: 15000,
            points_in_rank: 0,
            points_to_next: 15000,
            progress_percent: 0,
            trophy_key: 'clueless-wanderer',
        },
    },
    global_rank: {
        rank: 3,
        total_players: 1943,
    },
    global_average_rank: {
        rank: 7,
        total_players: 1943,
    },
    global_elo_rank: {
        rank: 5,
        total_players: 512,
    },
};

const renderProfile = (initialEntry = '/profile') =>
    render(
        <AuthContext.Provider value={authValue}>
            <MemoryRouter initialEntries={[initialEntry]}>
                <Routes>
                    <Route path="/profile" element={<ProfilePage />} />
                    <Route path="/profile/:userId" element={<ProfilePage />} />
                </Routes>
            </MemoryRouter>
        </AuthContext.Provider>,
    );

beforeEach(() => {
    vi.clearAllMocks();
    mocks.listBlocks.mockReset().mockResolvedValue({ items: [] });
    mocks.block.mockReset().mockResolvedValue(undefined);
    mocks.unblock.mockReset().mockResolvedValue(undefined);
    vi.stubGlobal(
        'confirm',
        vi.fn(() => true),
    );
    mocks.get.mockReset();
    mocks.profileLeaderboard.mockReset();
    mocks.get.mockResolvedValue({ data: { items: [], next_cursor: '' } });
    mocks.profileLeaderboard.mockResolvedValue({ items: [], next_cursor: '' });
});

describe('ProfilePage', () => {
    it('confirms a block and hides profile content only after success', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, id: 'user-2' } });
        renderProfile('/profile/user-2');
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
        const button = screen.getByRole('button', { name: 'Block player' });
        await waitFor(() => expect(button).toBeEnabled());
        vi.mocked(window.confirm).mockReturnValueOnce(false);
        fireEvent.click(button);
        expect(mocks.block).not.toHaveBeenCalled();
        mocks.block.mockRejectedValueOnce(new Error('Block failed'));
        fireEvent.click(button);
        expect(await screen.findByRole('alert')).toHaveTextContent('Block failed');
        expect(screen.getByRole('heading', { name: 'alice' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Retry blocked users' }));
        await waitFor(() => expect(button).toBeEnabled());
        fireEvent.click(button);
        expect(await screen.findByText('Player blocked')).toBeInTheDocument();
        expect(mocks.block).toHaveBeenCalledWith('user-2', expect.any(AbortSignal));
        expect(screen.queryByRole('heading', { name: 'alice' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Report player' })).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Unblock player' })).toBeEnabled();
    });

    it('unblocks an outgoing block even when the profile is unavailable', async () => {
        mocks.listBlocks.mockResolvedValue({
            items: [{ user_id: 'user-2', username: 'bob', avatar: 'avatar.png', created_at: '2026-01-01T00:00:00Z' }],
        });
        mocks.get
            .mockRejectedValueOnce(new Error('Not found'))
            .mockResolvedValueOnce({ data: { ...profile, id: 'user-2' } });
        renderProfile('/profile/user-2');
        fireEvent.click(await screen.findByRole('button', { name: 'Unblock player' }));
        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.unblock).toHaveBeenCalledWith('user-2', expect.any(AbortSignal));
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('shows an equipped map pin and the challenge that unlocked it', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                ...profile,
                map_pin: {
                    key: 'north-star',
                    name: 'North Star',
                    description: 'A clear sky marker.',
                    image_url: '/map-pins/north-star.svg',
                    unlocked_by: {
                        key: 'perfect-score',
                        name: 'Perfect score',
                        description: 'Get the maximum score once.',
                    },
                },
            },
        });
        renderProfile();

        expect(await screen.findByRole('heading', { name: 'North Star' })).toBeInTheDocument();
        expect(screen.getByText('Perfect score')).toBeInTheDocument();
        expect(screen.getByText('Get the maximum score once.')).toBeInTheDocument();
        expect(screen.getByRole('img', { name: "alice's North Star map pin" })).toHaveAttribute(
            'src',
            '/map-pins/north-star.svg',
        );
    });

    it('loads the profile and renders progression trackers with the next rank', async () => {
        mocks.get.mockResolvedValueOnce({ data: profile });
        renderProfile();

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(document.querySelector('.profile-map-pin__artwork img')).toHaveAttribute(
            'src',
            DEFAULT_MAP_PIN_IMAGE_URL,
        );
        expect(screen.getByText('6,000')).toBeInTheDocument();
        expect(screen.getByText('#3 of 1,943 players')).toBeInTheDocument();
        expect(screen.getByText('1500.0')).toBeInTheDocument();
        expect(screen.getByText('#7 of 1,943 players')).toBeInTheDocument();
        expect(screen.getByText('1,152')).toBeInTheDocument();
        expect(screen.getByText('#5 of 512 rated players')).toBeInTheDocument();
        expect(screen.getAllByText('Lost Tourist')).toHaveLength(2);
        expect(screen.getAllByText('II')).toHaveLength(4);
        expect(screen.getByText('III')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'Next rank: Clueless Wanderer' })).toBeInTheDocument();
        expect(screen.getByText(/9,000 to go/)).toBeInTheDocument();
        expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '10');
        // The hero avatar opens full screen.
        fireEvent.click(screen.getByRole('button', { name: "View alice's avatar full screen" }));
        expect(screen.getByRole('dialog', { name: "alice's avatar full screen" })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close full-screen photo' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        expect(screen.getByRole('img', { name: 'Lost Tourist badge' })).toHaveAttribute(
            'src',
            '/rank-badges/lost-tourist.png',
        );
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
        expect(mocks.get).toHaveBeenCalledWith('/auth/profile', { signal: expect.any(AbortSignal) });
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('shows an actionable error and retries the profile request', async () => {
        mocks.get
            .mockRejectedValueOnce(new Error('Profile service unavailable'))
            .mockResolvedValueOnce({ data: profile });
        renderProfile();

        expect(await screen.findByRole('alert')).toHaveTextContent('Profile service unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        await waitFor(() => expect(screen.getByRole('heading', { name: 'alice' })).toBeInTheDocument());
        // The profile mounts FeedLeaderboard after the retry. Wait for that
        // child request to settle before the test cleanup unmounts the tree;
        // otherwise a fast response can update state after the test ends.
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledTimes(2);
    });

    it('marks a player who never guessed as unranked', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                ...profile,
                total_points: 0,
                guess_count: 0,
                global_rank: { rank: 0, total_players: 1943 },
                global_average_rank: { rank: 0, total_players: 1943 },
                global_elo_rank: { rank: 0, total_players: 0 },
                elo: 0,
                average_score: 0,
            },
        });
        renderProfile();

        expect((await screen.findAllByText('Guess a group challenge to enter the ranking')).length).toBeGreaterThan(0);
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('shows persisted zero-point guesses without changing points or average', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                ...profile,
                total_points: 0,
                guess_count: 2,
                average_score: 0,
                global_rank: { rank: 0, total_players: 0 },
                global_average_rank: { rank: 0, total_players: 0 },
            },
        });
        renderProfile();

        const guesses = (await screen.findByText('Guesses made')).closest('.profile-stat-card');
        expect(guesses).toHaveTextContent('2');
        const points = screen.getByText('Total points').closest('.profile-stat-card');
        expect(points).toHaveTextContent('0');
        expect(screen.getByText('Average score').closest('.profile-stat-card')).toHaveTextContent('0.0');
        expect(screen.getAllByText('Guess a group challenge to enter the ranking')).toHaveLength(2);
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('loads another player public profile without account details', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, id: 'user-2', email: undefined } });
        renderProfile('/profile/user-2');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-2', { signal: expect.any(AbortSignal) });
        expect(screen.queryByText('alice@example.test')).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Settings' })).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Report player' })).toBeInTheDocument();
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('renders the feed leaderboard and loads the next page', async () => {
        mocks.get.mockResolvedValueOnce({ data: profile });
        mocks.profileLeaderboard
            .mockResolvedValueOnce({
                items: [{ rank: 1, user_id: 'user-2', username: 'bob', avatar: 'avatar-bob.png', total_score: 5000 }],
                next_cursor: 'next-page',
            })
            .mockResolvedValueOnce({
                items: [
                    { rank: 2, user_id: 'user-3', username: 'carol', avatar: 'avatar-carol.png', total_score: 4200 },
                ],
                next_cursor: '',
            });
        renderProfile();

        expect(await screen.findByRole('heading', { name: 'Best at guessing alice' })).toBeInTheDocument();
        expect(await screen.findByRole('link', { name: 'bob' })).toHaveAttribute('href', '/profile/user-2');
        fireEvent.click(screen.getByRole('button', { name: 'More players' }));
        expect(await screen.findByText('carol')).toBeInTheDocument();
        expect(mocks.profileLeaderboard).toHaveBeenLastCalledWith('user-1', 'next-page', expect.any(AbortSignal));
    });

    it('shows the edit link when viewing yourself through the public route', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, email: undefined } });
        renderProfile('/profile/user-1');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-1', { signal: expect.any(AbortSignal) });
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
        expect(screen.queryByRole('button', { name: 'Report player' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Block player' })).not.toBeInTheDocument();
        expect(mocks.listBlocks).not.toHaveBeenCalled();
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });

    it('shows only the verified recovery email on the owner profile', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                ...profile,
                email: 'alice@example.test',
                email_verified_at: '2026-01-01T00:00:00Z',
                pending_email: 'new@example.test',
            },
        });
        renderProfile();

        expect(await screen.findByText('alice@example.test')).toBeInTheDocument();
        // A pending claim is never shown on a profile view; it is managed in
        // account settings.
        expect(screen.queryByText('new@example.test')).not.toBeInTheDocument();
        expect(await screen.findByRole('heading', { name: 'No scores yet' })).toBeInTheDocument();
    });
});
