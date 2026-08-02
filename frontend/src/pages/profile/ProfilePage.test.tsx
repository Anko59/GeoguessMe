import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import type { User } from '../../types';
import ProfilePage from './ProfilePage';

const mocks = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    email_verified_at: null,
    avatar: 'avatar.png',
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
    total_points: 600,
    guess_count: 4,
    rank: {
        level: 2,
        name: 'Squire',
        min_points: 500,
        next_points: 1500,
        points_in_rank: 100,
        points_to_next: 1000,
        progress_percent: 10,
        trophy_key: 'squire',
        next_rank: {
            level: 3,
            name: 'Yeoman',
            min_points: 1500,
            points_in_rank: 0,
            points_to_next: 1500,
            progress_percent: 0,
            trophy_key: 'yeoman',
        },
    },
    global_rank: {
        rank: 3,
        total_players: 1943,
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
    mocks.get.mockReset();
});

describe('ProfilePage', () => {
    it('loads the profile and renders progression trackers with the next rank', async () => {
        mocks.get.mockResolvedValueOnce({ data: profile });
        renderProfile();

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(screen.getByText('600')).toBeInTheDocument();
        expect(screen.getAllByText('Squire')).toHaveLength(2);
        expect(screen.getAllByText('II')).toHaveLength(4);
        expect(screen.getByText('III')).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'Next rank: Yeoman' })).toBeInTheDocument();
        expect(screen.getByText(/900 to go/)).toBeInTheDocument();
        expect(screen.getByText('of 1,943 players')).toBeInTheDocument();
        expect(screen.getByText('#3')).toBeInTheDocument();
        expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '10');
        expect(screen.getByRole('img', { name: 'Squire badge' })).toHaveAttribute('src', '/rank-badges/squire.png');
        expect(screen.getByRole('link', { name: 'Edit profile' })).toHaveAttribute('href', '/settings');
        expect(mocks.get).toHaveBeenCalledWith('/auth/profile');
    });

    it('shows an actionable error and retries the profile request', async () => {
        mocks.get
            .mockRejectedValueOnce(new Error('Profile service unavailable'))
            .mockResolvedValueOnce({ data: profile });
        renderProfile();

        expect(await screen.findByRole('alert')).toHaveTextContent('Profile service unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        await waitFor(() => expect(screen.getByRole('heading', { name: 'alice' })).toBeInTheDocument());
        expect(mocks.get).toHaveBeenCalledTimes(2);
    });

    it('marks a player who never guessed as unranked', async () => {
        mocks.get.mockResolvedValueOnce({
            data: { ...profile, total_points: 0, guess_count: 0, global_rank: { rank: 0, total_players: 1943 } },
        });
        renderProfile();

        expect(await screen.findByText('Unranked')).toBeInTheDocument();
        expect(screen.getByText('Guess a location to enter the ranking')).toBeInTheDocument();
    });

    it('loads another player public profile without account details', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, email: undefined } });
        renderProfile('/profile/user-2');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-2');
        expect(screen.queryByText('alice@example.test')).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Edit profile' })).not.toBeInTheDocument();
    });

    it('shows the edit link when viewing yourself through the public route', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, email: undefined } });
        renderProfile('/profile/user-1');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-1');
        expect(screen.getByRole('link', { name: 'Edit profile' })).toHaveAttribute('href', '/settings');
    });
});
