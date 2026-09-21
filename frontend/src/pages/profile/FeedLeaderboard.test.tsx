import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import type { User } from '../../types';
import FeedLeaderboard from './FeedLeaderboard';

const mocks = vi.hoisted(() => ({ leaderboard: vi.fn() }));

vi.mock('../../api', () => ({
    publicFeedAPI: { leaderboard: mocks.leaderboard },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    email_verified_at: null,
    password_login_enabled: true,
    oidc_linked: false,
    migration_required: false,
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

const renderLeaderboard = () =>
    render(
        <AuthContext.Provider value={authValue}>
            <MemoryRouter>
                <FeedLeaderboard />
            </MemoryRouter>
        </AuthContext.Provider>,
    );

beforeEach(() => {
    vi.clearAllMocks();
    mocks.leaderboard.mockReset();
});

describe('FeedLeaderboard', () => {
    it('uses the group leaderboard presentation while retaining feed cursor pagination', async () => {
        mocks.leaderboard
            .mockResolvedValueOnce({
                items: [
                    { rank: 1, user_id: 'user-1', username: 'alice', total_score: 5000 },
                    { rank: 2, user_id: 'user-2', username: 'bob', total_score: 4000 },
                ],
                next_cursor: 'next-page',
            })
            .mockResolvedValueOnce({
                items: [{ rank: 3, user_id: 'user-3', username: 'carol', total_score: 2500 }],
                next_cursor: '',
            });

        renderLeaderboard();

        expect(await screen.findByRole('heading', { name: 'Feed leaderboard' })).toBeInTheDocument();
        expect(screen.getByText('Community rankings')).toBeInTheDocument();
        expect(screen.getByText('All-time totals')).toBeInTheDocument();
        expect(screen.getByRole('list', { name: 'Feed leaderboard rankings' })).toHaveClass('leaderboard-list');

        const aliceRow = screen.getAllByRole('listitem')[0];
        expect(aliceRow).toHaveClass('leaderboard-entry', 'gold', 'current-user');
        expect(aliceRow.querySelector('.entry-score-bar')).toBeInTheDocument();
        expect(aliceRow.querySelector('.score-fill')).toHaveStyle({ width: '100%' });
        expect(screen.getByRole('link', { name: "View alice's profile" })).toHaveAttribute('href', '/profile/user-1');
        expect(screen.getByText('5,000')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'More players' }));
        expect(await screen.findByText('carol')).toBeInTheDocument();
        expect(mocks.leaderboard).toHaveBeenLastCalledWith('next-page', expect.any(AbortSignal));
        expect(screen.queryByRole('button', { name: 'More players' })).not.toBeInTheDocument();
    });

    it('keeps a retry action for an unavailable feed leaderboard', async () => {
        mocks.leaderboard
            .mockRejectedValueOnce(new Error('rankings unavailable'))
            .mockResolvedValueOnce({ items: [], next_cursor: '' });

        renderLeaderboard();

        expect(await screen.findByRole('alert')).toHaveTextContent('rankings unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        await waitFor(() => expect(screen.getByText('No scores yet')).toBeInTheDocument());
        expect(mocks.leaderboard).toHaveBeenCalledTimes(2);
        expect(mocks.leaderboard).toHaveBeenLastCalledWith('', expect.any(AbortSignal));
    });
});
