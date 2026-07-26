import { beforeEach, describe, expect, it, vi } from 'vitest';
import { getCachedLeaderboard, refreshLeaderboard } from './leaderboardCache';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

describe('leaderboardCache', () => {
    beforeEach(() => {
        mocks.get.mockReset();
    });

    it('deduplicates concurrent requests and keeps results scoped to the signed-in user', async () => {
        let resolve!: (value: {
            data: Array<{
                user_id: string;
                username: string;
                score: number;
                guess_count: number;
                average_score: number;
            }>;
        }) => void;
        mocks.get.mockReturnValue(
            new Promise((done) => {
                resolve = done;
            }),
        );

        const first = refreshLeaderboard('user-a', 'group-a', 'week');
        const second = refreshLeaderboard('user-a', 'group-a', 'week');
        expect(mocks.get).toHaveBeenCalledTimes(1);
        expect(mocks.get).toHaveBeenCalledWith('/group/leaderboard', {
            params: { group_id: 'group-a', period: 'week' },
        });

        resolve({ data: [{ user_id: 'user-a', username: 'alice', score: 10, guess_count: 1, average_score: 10 }] });
        await expect(first).resolves.toEqual(await second);
        expect(getCachedLeaderboard('user-a', 'group-a', 'week')).toEqual([
            { user_id: 'user-a', username: 'alice', score: 10, guess_count: 1, average_score: 10 },
        ]);
        expect(getCachedLeaderboard('user-b', 'group-a', 'week')).toBeUndefined();
    });
});
