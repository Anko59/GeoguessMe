import api from '../../api';
import type { LeaderboardEntry, LeaderboardMetric, LeaderboardPeriod } from '../../types';

const entries = new Map<string, LeaderboardEntry[]>();
const requests = new Map<string, Promise<LeaderboardEntry[]>>();

function keyFor(userID: string, groupID: string, period: LeaderboardPeriod, metric: LeaderboardMetric): string {
    return `${userID}:${groupID}:${period}:${metric}`;
}

export function getCachedLeaderboard(
    userID: string | undefined,
    groupID: string,
    period: LeaderboardPeriod,
    metric: LeaderboardMetric,
): LeaderboardEntry[] | undefined {
    return userID ? entries.get(keyFor(userID, groupID, period, metric)) : undefined;
}

export async function refreshLeaderboard(
    userID: string,
    groupID: string,
    period: LeaderboardPeriod,
    metric: LeaderboardMetric,
): Promise<LeaderboardEntry[]> {
    const key = keyFor(userID, groupID, period, metric);
    const existing = requests.get(key);
    if (existing) return existing;

    const request = api
        .get<LeaderboardEntry[]>('/group/leaderboard', { params: { group_id: groupID, period, metric } })
        .then((response) => response.data || [])
        .then((leaderboard) => {
            entries.set(key, leaderboard);
            return leaderboard;
        })
        .finally(() => requests.delete(key));
    requests.set(key, request);
    return request;
}

// This is intentionally best-effort: a prefetch must never create an error
// state before the user explicitly opens the leaderboard.
export function prefetchLeaderboard(userID: string, groupID: string): void {
    void refreshLeaderboard(userID, groupID, 'week', 'total').catch(() => undefined);
}
