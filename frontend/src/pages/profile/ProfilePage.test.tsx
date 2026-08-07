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
    mocks.get.mockReset();
});

describe('ProfilePage', () => {
    it('loads the profile and renders progression trackers with the next rank', async () => {
        mocks.get.mockResolvedValueOnce({ data: profile });
        renderProfile();

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
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

        expect((await screen.findAllByText('Guess a location to enter the ranking')).length).toBeGreaterThan(0);
    });

    it('loads another player public profile without account details', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, email: undefined } });
        renderProfile('/profile/user-2');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-2');
        expect(screen.queryByText('alice@example.test')).not.toBeInTheDocument();
        expect(screen.queryByRole('link', { name: 'Settings' })).not.toBeInTheDocument();
    });

    it('shows the edit link when viewing yourself through the public route', async () => {
        mocks.get.mockResolvedValueOnce({ data: { ...profile, email: undefined } });
        renderProfile('/profile/user-1');

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/user/profile/user-1');
        expect(screen.getByRole('link', { name: 'Settings' })).toHaveAttribute('href', '/settings');
    });
});
