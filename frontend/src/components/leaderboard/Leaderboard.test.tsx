import { act, fireEvent, render, screen } from '@testing-library/react';
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
};

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
});

describe('Leaderboard', () => {
    it('renders leaderboard loading, empty, error, and ranked states', async () => {
        mocks.get.mockResolvedValueOnce({ data: [] });
        let emptyLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            emptyLeaderboard = render(
                <AuthContext.Provider value={authValue}>
                    <Leaderboard groupID="group-1" />
                </AuthContext.Provider>,
            );
        });
        expect(await screen.findByText('No scores yet')).toBeInTheDocument();
        emptyLeaderboard!.unmount();

        mocks.get.mockResolvedValueOnce({
            data: [
                { user_id: 'user-1', username: 'alice', score: 100, guess_count: 1, average_score: 100 },
                { user_id: 'user-2', username: 'bob', score: 80, guess_count: 1, average_score: 80 },
                { user_id: 'user-3', username: 'eve', score: 60, guess_count: 1, average_score: 60 },
                { user_id: 'user-4', username: 'dan', score: 40, guess_count: 1, average_score: 40 },
            ],
        });
        let rankedLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            rankedLeaderboard = render(
                <AuthContext.Provider value={authValue}>
                    <Leaderboard groupID="group-1" />
                </AuthContext.Provider>,
            );
        });
        expect(await screen.findByText('alice')).toBeInTheDocument();
        expect(screen.getByText('You')).toBeInTheDocument();
        expect(screen.getByText('#4')).toBeInTheDocument();
        rankedLeaderboard!.unmount();

        mocks.get.mockRejectedValueOnce(new Error('rankings unavailable'));
        let errorLeaderboard: ReturnType<typeof render>;
        await act(async () => {
            errorLeaderboard = render(
                <AuthContext.Provider value={authValue}>
                    <Leaderboard groupID="group-1" />
                </AuthContext.Provider>,
            );
        });
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to load rankings');
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        });
        errorLeaderboard!.unmount();
    });
});
