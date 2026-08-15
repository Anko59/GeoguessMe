import api from '../../api';
import type { LeaderboardEntry, LeaderboardMetric, LeaderboardPeriod } from '../../types';

// The leaderboard cache is bounded: entries expire after LEADERBOARD_TTL_MS
// and the map never holds more than MAX_LEADERBOARD_ENTRIES entries (oldest
// insertion first). The Leaderboard view polls every 10s, so the TTL only
// prevents serving data older than a minute on a fresh mount.
const LEADERBOARD_TTL_MS = 60_000;
const MAX_LEADERBOARD_ENTRIES = 50;

interface CachedLeaderboard {
    leaderboard: LeaderboardEntry[];
    storedAt: number;
}

const cache = new Map<string, CachedLeaderboard>();
const requests = new Map<string, Promise<LeaderboardEntry[]>>();

function keyFor(userID: string, groupID: string, period: LeaderboardPeriod, metric: LeaderboardMetric): string {
    return `${userID}:${groupID}:${period}:${metric}`;
}

/** Drop expired entries and evict the oldest when the cache exceeds its cap. */
function prune(now: number): void {
    for (const [key, entry] of cache) {
        if (now - entry.storedAt > LEADERBOARD_TTL_MS) cache.delete(key);
    }
    while (cache.size > MAX_LEADERBOARD_ENTRIES) {
        // Map iteration yields insertion order; the first key is the oldest.
        const oldest = cache.keys().next().value;
        if (oldest === undefined) break;
        cache.delete(oldest);
    }
}

export function getCachedLeaderboard(
    userID: string | undefined,
    groupID: string,
    period: LeaderboardPeriod,
    metric: LeaderboardMetric,
): LeaderboardEntry[] | undefined {
    if (!userID) return undefined;
    prune(Date.now());
    return cache.get(keyFor(userID, groupID, period, metric))?.leaderboard;
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
            const now = Date.now();
            prune(now);
            cache.set(key, { leaderboard, storedAt: now });
            return leaderboard;
        })
        .finally(() => requests.delete(key));
    requests.set(key, request);
    return request;
}

/** Drop every cached leaderboard (the explicit invalidation path; also used
 *  to isolate cache tests). */
export function clearLeaderboardCache(): void {
    cache.clear();
    requests.clear();
}

// This is intentionally best-effort: a prefetch must never create an error
// state before the user explicitly opens the leaderboard.
export function prefetchLeaderboard(userID: string, groupID: string): void {
    void refreshLeaderboard(userID, groupID, 'week', 'total').catch(() => undefined);
}
