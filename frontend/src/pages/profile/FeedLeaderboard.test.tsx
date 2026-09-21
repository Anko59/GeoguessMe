import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import FeedLeaderboard from './FeedLeaderboard';

const mocks = vi.hoisted(() => ({ profileLeaderboard: vi.fn() }));

vi.mock('../../api', () => ({
    publicFeedAPI: { profileLeaderboard: mocks.profileLeaderboard },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

const renderLeaderboard = () =>
    render(
        <MemoryRouter>
            <FeedLeaderboard profileID="profile-1" profileUsername="alice" />
        </MemoryRouter>,
    );

beforeEach(() => {
    vi.clearAllMocks();
    mocks.profileLeaderboard.mockReset();
});

describe('FeedLeaderboard', () => {
    it('uses the group leaderboard presentation while retaining feed cursor pagination', async () => {
        mocks.profileLeaderboard
            .mockResolvedValueOnce({
                items: [
                    { rank: 1, user_id: 'user-1', username: 'alice', avatar: 'avatar.png', total_score: 5000 },
                    { rank: 2, user_id: 'user-2', username: 'bob', avatar: 'avatar.png', total_score: 4000 },
                ],
                next_cursor: 'next-page',
            })
            .mockResolvedValueOnce({
                items: [
                    { rank: 2, user_id: 'user-2', username: 'bob', avatar: 'avatar.png', total_score: 4000 },
                    { rank: 3, user_id: 'user-3', username: 'carol', avatar: 'avatar.png', total_score: 2500 },
                ],
                next_cursor: '',
            });

        renderLeaderboard();

        expect(await screen.findByRole('heading', { name: 'Best at guessing alice' })).toBeInTheDocument();
        expect(screen.getByText('Challenge rankings')).toBeInTheDocument();
        expect(screen.getByText('All-time totals')).toBeInTheDocument();
        expect(screen.getByRole('list', { name: 'Feed leaderboard rankings' })).toHaveClass('leaderboard-list');

        const aliceRow = screen.getAllByRole('listitem')[0];
        expect(aliceRow).toHaveClass('leaderboard-entry', 'gold');
        expect(aliceRow).not.toHaveClass('current-user');
        expect(aliceRow.querySelector('.entry-score-bar')).toBeInTheDocument();
        expect(aliceRow.querySelector('.score-fill')).toHaveStyle({ width: '100%' });
        expect(screen.getByRole('link', { name: "View alice's profile" })).toHaveAttribute('href', '/profile/user-1');
        expect(screen.getByText('5,000')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'More players' }));
        expect(await screen.findByText('carol')).toBeInTheDocument();
        expect(screen.getAllByRole('listitem')).toHaveLength(3);
        expect(mocks.profileLeaderboard).toHaveBeenLastCalledWith('profile-1', 'next-page', expect.any(AbortSignal));
        expect(screen.queryByRole('button', { name: 'More players' })).not.toBeInTheDocument();
    });

    it('keeps a retry action for an unavailable feed leaderboard', async () => {
        mocks.profileLeaderboard
            .mockRejectedValueOnce(new Error('rankings unavailable'))
            .mockResolvedValueOnce({ items: [], next_cursor: '' });

        renderLeaderboard();

        expect(await screen.findByRole('alert')).toHaveTextContent('rankings unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        await waitFor(() => expect(screen.getByText('No scores yet')).toBeInTheDocument());
        expect(mocks.profileLeaderboard).toHaveBeenCalledTimes(2);
        expect(mocks.profileLeaderboard).toHaveBeenLastCalledWith('profile-1', '', expect.any(AbortSignal));
    });
});
