import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import Leaderboard from './Leaderboard';
import type { User } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
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

const pageRank = {
    level: 1,
    name: 'Completely Lost',
    min_points: 0,
    points_in_rank: 0,
    points_to_next: 5000,
    progress_percent: 0,
    trophy_key: 'completely-lost',
};

const renderLeaderboard = (groupID = 'group-1') =>
    render(
        <AuthContext.Provider value={authValue}>
            <MemoryRouter>
                <Leaderboard groupID={groupID} />
            </MemoryRouter>
        </AuthContext.Provider>,
    );

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
});

describe('Leaderboard', () => {
    it('renders leaderboard loading, empty, error, and ranked states', async () => {
        mocks.get.mockResolvedValueOnce({ data: [] });
        let emptyLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            emptyLeaderboard = renderLeaderboard();
        });
        expect(await screen.findByText('No scores yet')).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/group/leaderboard', {
            params: { group_id: 'group-1', period: 'week', metric: 'total' },
        });
        emptyLeaderboard!.unmount();

        mocks.get.mockResolvedValueOnce({
            data: [
                {
                    user_id: 'user-1',
                    username: 'alice',
                    avatar: 'avatar2.png',
                    score: 100,
                    guess_count: 1,
                    average_score: 100,
                    total_points: 100,
                    elo: 1000,
                    rank: pageRank,
                },
                {
                    user_id: 'user-2',
                    username: 'bob',
                    avatar: 'avatar3.png',
                    score: 80,
                    guess_count: 1,
                    average_score: 80,
                    total_points: 80,
                    elo: 950,
                    rank: pageRank,
                },
                {
                    user_id: 'user-3',
                    username: 'eve',
                    avatar: 'avatar4.png',
                    score: 60,
                    guess_count: 1,
                    average_score: 60,
                    total_points: 60,
                    elo: 900,
                    rank: pageRank,
                },
                {
                    user_id: 'user-4',
                    username: 'dan',
                    avatar: 'avatar5.png',
                    score: 40,
                    guess_count: 1,
                    average_score: 40,
                    total_points: 40,
                    elo: 850,
                    rank: { ...pageRank, level: 17, name: 'Cartographer', trophy_key: 'cartographer' },
                },
            ],
        });
        let rankedLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            rankedLeaderboard = renderLeaderboard();
        });
        expect(await screen.findByText('alice')).toBeInTheDocument();
        expect(screen.getByText('You')).toBeInTheDocument();
        expect(screen.getAllByText('Completely Lost')).toHaveLength(3);
        expect(screen.getByText('Cartographer')).toBeInTheDocument();
        expect(screen.getAllByText('I')).toHaveLength(3);
        expect(screen.getByText('XVII')).toBeInTheDocument();
        expect(screen.getByText('#4')).toBeInTheDocument();
        expect(screen.getByRole('img', { name: 'alice' })).toHaveAttribute('src', '/avatars/avatar2.png');
        expect(screen.getByRole('link', { name: 'alice' })).toHaveAttribute('href', '/profile/user-1');
        expect(screen.getByRole('link', { name: "View alice's profile" })).toHaveAttribute('href', '/profile/user-1');
        const badges = rankedLeaderboard!.container.querySelectorAll('.rank-badge');
        expect(badges).toHaveLength(4);
        expect((badges[3] as HTMLImageElement).src).toContain('/rank-badges/cartographer.png');
        rankedLeaderboard!.unmount();

        mocks.get.mockRejectedValueOnce(new Error('rankings unavailable'));
        let errorLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            errorLeaderboard = renderLeaderboard();
        });
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load rankings');
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        });
        errorLeaderboard!.unmount();
    });

    it('switches between weekly, monthly, and all-time rankings', async () => {
        mocks.get
            .mockResolvedValueOnce({
                data: [
                    {
                        user_id: 'user-1',
                        username: 'alice',
                        avatar: 'avatar2.png',
                        score: 100,
                        guess_count: 1,
                        average_score: 100,
                        total_points: 100,
                        rank: pageRank,
                    },
                ],
            })
            .mockResolvedValueOnce({
                data: [
                    {
                        user_id: 'user-2',
                        username: 'bob',
                        avatar: 'avatar3.png',
                        score: 80,
                        guess_count: 1,
                        average_score: 80,
                        total_points: 80,
                        rank: pageRank,
                    },
                ],
            });
        renderLeaderboard();

        expect(await screen.findByText('alice')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('tab', { name: 'This month' }));

        expect(await screen.findByText('bob')).toBeInTheDocument();
        expect(mocks.get).toHaveBeenLastCalledWith('/group/leaderboard', {
            params: { group_id: 'group-1', period: 'month', metric: 'total' },
        });
        expect(screen.getByRole('tab', { name: 'This month' })).toHaveAttribute('aria-selected', 'true');
    });

    it('switches between total, average, and elo rankings', async () => {
        const entry = {
            user_id: 'user-1',
            username: 'alice',
            avatar: 'avatar2.png',
            score: 100,
            guess_count: 2,
            average_score: 50,
            total_points: 100,
            elo: 1120,
            rank: pageRank,
        };
        mocks.get
            .mockResolvedValueOnce({ data: [entry] })
            .mockResolvedValueOnce({ data: [{ ...entry, username: 'bob' }] })
            .mockResolvedValueOnce({
                data: [
                    { ...entry, username: 'carol' },
                    { ...entry, user_id: 'user-2', username: 'dave', elo: 0 },
                ],
            });
        renderLeaderboard();

        expect(await screen.findByText('alice')).toBeInTheDocument();
        expect(screen.getByText('100')).toBeInTheDocument();
        expect(screen.getByText('pts')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('tab', { name: 'Average' }));
        expect(await screen.findByText('bob')).toBeInTheDocument();
        expect(mocks.get).toHaveBeenLastCalledWith('/group/leaderboard', {
            params: { group_id: 'group-1', period: 'week', metric: 'average' },
        });
        expect(screen.getByText('50.0')).toBeInTheDocument();
        expect(screen.getByText('avg')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('tab', { name: 'Elo' }));
        expect(await screen.findByText('carol')).toBeInTheDocument();
        expect(mocks.get).toHaveBeenLastCalledWith('/group/leaderboard', {
            params: { group_id: 'group-1', period: 'week', metric: 'elo' },
        });
        expect(screen.getByText('1,120')).toBeInTheDocument();
        expect(screen.getAllByText('elo')).toHaveLength(2);
        expect(screen.getByRole('tab', { name: 'Elo' })).toHaveAttribute('aria-selected', 'true');
        const unratedRow = screen.getByText('dave').closest('.leaderboard-entry');
        expect(unratedRow?.querySelector('.entry-rank')).toHaveTextContent('—');
        expect(unratedRow).not.toHaveClass('gold', 'silver', 'bronze');
    });
});
