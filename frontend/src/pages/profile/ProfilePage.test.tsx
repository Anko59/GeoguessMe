import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import ProfilePage from './ProfilePage';

const mocks = vi.hoisted(() => ({ get: vi.fn() }));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

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
    },
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
});

describe('ProfilePage', () => {
    it('loads the profile and renders progression trackers', async () => {
        mocks.get.mockResolvedValueOnce({ data: profile });
        render(
            <MemoryRouter>
                <ProfilePage />
            </MemoryRouter>,
        );

        expect(await screen.findByRole('heading', { name: 'alice' })).toBeInTheDocument();
        expect(screen.getByText('600')).toBeInTheDocument();
        expect(screen.getAllByText('Squire')).toHaveLength(2);
        expect(screen.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '10');
        expect(screen.getByRole('img', { name: 'Squire trophy' })).toBeInTheDocument();
        expect(mocks.get).toHaveBeenCalledWith('/auth/profile');
    });

    it('shows an actionable error and retries the profile request', async () => {
        mocks.get
            .mockRejectedValueOnce(new Error('Profile service unavailable'))
            .mockResolvedValueOnce({ data: profile });
        render(
            <MemoryRouter>
                <ProfilePage />
            </MemoryRouter>,
        );

        expect(await screen.findByRole('alert')).toHaveTextContent('Profile service unavailable');
        fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
        await waitFor(() => expect(screen.getByRole('heading', { name: 'alice' })).toBeInTheDocument());
        expect(mocks.get).toHaveBeenCalledTimes(2);
    });
});
