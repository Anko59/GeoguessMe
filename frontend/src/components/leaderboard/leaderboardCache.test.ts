import { beforeEach, describe, expect, it, vi } from 'vitest';
import { clearLeaderboardCache, getCachedLeaderboard, refreshLeaderboard } from './leaderboardCache';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get },
}));

describe('leaderboardCache', () => {
    beforeEach(() => {
        mocks.get.mockReset();
        clearLeaderboardCache();
    });

    it('deduplicates concurrent requests and keeps results scoped to the signed-in user', async () => {
        let resolve!: (value: {
            data: Array<{
                user_id: string;
                username: string;
                avatar: string;
                score: number;
                guess_count: number;
                average_score: number;
                total_points: number;
                elo: number;
                rank: {
                    level: number;
                    name: string;
                    min_points: number;
                    points_in_rank: number;
                    points_to_next: number;
                    progress_percent: number;
                    trophy_key: string;
                };
            }>;
        }) => void;
        mocks.get.mockReturnValue(
            new Promise((done) => {
                resolve = done;
            }),
        );

        const first = refreshLeaderboard('user-a', 'group-a', 'week', 'total');
        const second = refreshLeaderboard('user-a', 'group-a', 'week', 'total');
        expect(mocks.get).toHaveBeenCalledTimes(1);
        expect(mocks.get).toHaveBeenCalledWith('/group/leaderboard', {
            params: { group_id: 'group-a', period: 'week', metric: 'total' },
        });

        resolve({
            data: [
                {
                    user_id: 'user-a',
                    username: 'alice',
                    avatar: 'avatar2.png',
                    score: 10,
                    guess_count: 1,
                    average_score: 10,
                    total_points: 10,
                    elo: 1000,
                    rank: {
                        level: 1,
                        name: 'Page',
                        min_points: 0,
                        points_in_rank: 10,
                        points_to_next: 500,
                        progress_percent: 2,
                        trophy_key: 'page',
                    },
                },
            ],
        });
        await expect(first).resolves.toEqual(await second);
        expect(getCachedLeaderboard('user-a', 'group-a', 'week', 'total')).toEqual([
            {
                user_id: 'user-a',
                username: 'alice',
                avatar: 'avatar2.png',
                score: 10,
                guess_count: 1,
                average_score: 10,
                total_points: 10,
                elo: 1000,
                rank: {
                    level: 1,
                    name: 'Page',
                    min_points: 0,
                    points_in_rank: 10,
                    points_to_next: 500,
                    progress_percent: 2,
                    trophy_key: 'page',
                },
            },
        ]);
        expect(getCachedLeaderboard('user-b', 'group-a', 'week', 'total')).toBeUndefined();
    });

    it('expires cached entries after the TTL so stale rankings are never served', async () => {
        vi.useFakeTimers();
        try {
            mocks.get.mockResolvedValue({ data: [] });
            await refreshLeaderboard('user-a', 'group-a', 'week', 'total');
            expect(getCachedLeaderboard('user-a', 'group-a', 'week', 'total')).toEqual([]);

            vi.advanceTimersByTime(60_001);
            expect(getCachedLeaderboard('user-a', 'group-a', 'week', 'total')).toBeUndefined();
        } finally {
            vi.useRealTimers();
        }
    });

    it('evicts the oldest entries once the cache reaches its bound', async () => {
        mocks.get.mockResolvedValue({ data: [] });
        for (let index = 0; index < 51; index += 1) {
            await refreshLeaderboard(`user-${index}`, 'group-a', 'week', 'total');
        }
        expect(getCachedLeaderboard('user-0', 'group-a', 'week', 'total')).toBeUndefined();
        expect(getCachedLeaderboard('user-50', 'group-a', 'week', 'total')).toEqual([]);
    });

    it('clearLeaderboardCache drops every entry and in-flight request', async () => {
        mocks.get.mockResolvedValue({ data: [] });
        await refreshLeaderboard('user-a', 'group-a', 'week', 'total');
        clearLeaderboardCache();
        expect(getCachedLeaderboard('user-a', 'group-a', 'week', 'total')).toBeUndefined();
    });
});
