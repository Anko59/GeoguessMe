import { useCallback, useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { getAPIErrorMessage, publicFeedAPI } from '../../api';
import Avatar from '../../components/common/Avatar';
import type { PublicFeedLeaderboardEntry } from '../../types';
import '../../components/leaderboard/Leaderboard.css';

type LeaderboardState = {
    items: PublicFeedLeaderboardEntry[];
    nextCursor: string;
    error: string;
    loading: boolean;
};

function appendUniqueEntries(
    current: PublicFeedLeaderboardEntry[],
    additions: PublicFeedLeaderboardEntry[],
): PublicFeedLeaderboardEntry[] {
    const seen = new Set(current.map((entry) => entry.user_id));
    return additions.reduce<PublicFeedLeaderboardEntry[]>(
        (result, entry) => {
            if (!seen.has(entry.user_id)) {
                seen.add(entry.user_id);
                result.push(entry);
            }
            return result;
        },
        [...current],
    );
}

export default function FeedLeaderboard({
    profileID,
    profileUsername,
}: {
    profileID: string;
    profileUsername: string;
}) {
    const [state, setState] = useState<LeaderboardState>({ items: [], nextCursor: '', error: '', loading: true });
    const mounted = useRef(false);
    const initialLoadController = useRef<AbortController | null>(null);
    const loadMoreController = useRef<AbortController | null>(null);

    const loadInitial = useCallback(async () => {
        initialLoadController.current?.abort();
        const controller = new AbortController();
        initialLoadController.current = controller;
        setState((current) => ({ ...current, loading: true, error: '' }));
        try {
            const page = await publicFeedAPI.profileLeaderboard(profileID, '', controller.signal);
            if (mounted.current && !controller.signal.aborted && initialLoadController.current === controller) {
                setState({ items: page.items, nextCursor: page.next_cursor, error: '', loading: false });
            }
        } catch (reason: unknown) {
            if (mounted.current && !controller.signal.aborted && initialLoadController.current === controller) {
                setState({
                    items: [],
                    nextCursor: '',
                    error: getAPIErrorMessage(reason, 'Unable to load the feed leaderboard.'),
                    loading: false,
                });
            }
        } finally {
            if (initialLoadController.current === controller) {
                initialLoadController.current = null;
            }
        }
    }, [profileID]);

    useEffect(() => {
        mounted.current = true;
        void loadInitial();
        return () => {
            mounted.current = false;
            initialLoadController.current?.abort();
            loadMoreController.current?.abort();
        };
    }, [loadInitial]);

    async function loadMore() {
        if (!state.nextCursor || state.loading) return;
        loadMoreController.current?.abort();
        const controller = new AbortController();
        loadMoreController.current = controller;
        setState((current) => ({ ...current, loading: true, error: '' }));
        try {
            const page = await publicFeedAPI.profileLeaderboard(profileID, state.nextCursor, controller.signal);
            if (mounted.current && !controller.signal.aborted && loadMoreController.current === controller) {
                setState((current) => ({
                    items: appendUniqueEntries(current.items, page.items),
                    nextCursor: page.next_cursor,
                    error: '',
                    loading: false,
                }));
            }
        } catch (reason: unknown) {
            if (mounted.current && !controller.signal.aborted && loadMoreController.current === controller) {
                setState((current) => ({
                    ...current,
                    error: getAPIErrorMessage(reason, 'Unable to load more rankings.'),
                    loading: false,
                }));
            }
        } finally {
            if (loadMoreController.current === controller) {
                loadMoreController.current = null;
            }
        }
    }

    const getRankMedal = (rank: number) => {
        switch (rank) {
            case 1:
                return '/ui/medal-gold.png';
            case 2:
                return '/ui/medal-silver.png';
            case 3:
                return '/ui/medal-bronze.png';
            default:
                return null;
        }
    };

    const getRankClass = (rank: number) => {
        switch (rank) {
            case 1:
                return 'gold';
            case 2:
                return 'silver';
            case 3:
                return 'bronze';
            default:
                return '';
        }
    };

    const retry = () => {
        if (state.items.length > 0) {
            void loadMore();
        } else {
            void loadInitial();
        }
    };
    const leaderValue = state.items[0]?.total_score ?? 1;

    return (
        <section className="leaderboard-container" aria-labelledby="profile-feed-leaderboard-title">
            <div className="leaderboard-header">
                <img src="/friends_leaderboard_icon.png" alt="" className="leaderboard-icon" />
                <div>
                    <p>Challenge rankings</p>
                    <h2 id="profile-feed-leaderboard-title">Best at guessing {profileUsername}</h2>
                </div>
                <span className="leaderboard-scope">All-time totals</span>
            </div>
            {state.error && (
                <div className="leaderboard-error" role="alert">
                    <p>{state.error}</p>
                    <button className="btn btn-secondary" onClick={retry}>
                        Retry
                    </button>
                </div>
            )}
            {!state.error && state.loading && state.items.length === 0 && (
                <div className="loading-state" role="status">
                    <div className="spinner" />
                    <p>Loading feed rankings…</p>
                </div>
            )}
            {!state.error && !state.loading && state.items.length === 0 && (
                <div className="leaderboard-empty-state">
                    <img src="/cup_icon.png" alt="" className="leaderboard-empty-icon" />
                    <h2>No scores yet</h2>
                    <p className="empty-subtitle">
                        Be the first to score one of {profileUsername}&apos;s visible challenges.
                    </p>
                </div>
            )}
            {state.items.length > 0 && (
                <ol className="leaderboard-list" aria-label="Feed leaderboard rankings">
                    {state.items.map((entry, index) => {
                        const rankMedal = getRankMedal(entry.rank);
                        const rankClass = getRankClass(entry.rank);

                        return (
                            <li
                                key={entry.user_id}
                                className={`leaderboard-entry ${rankClass} scale-in`}
                                style={{ animationDelay: `${index * 0.05}s` }}
                            >
                                <div className="entry-rank">
                                    {rankMedal ? (
                                        <img src={rankMedal} alt="" className="entry-rank-medal" />
                                    ) : (
                                        `#${entry.rank}`
                                    )}
                                </div>
                                <div className="entry-avatar">
                                    <Link
                                        to={`/profile/${entry.user_id}`}
                                        aria-label={`View ${entry.username}'s profile`}
                                    >
                                        <Avatar
                                            userID={entry.user_id}
                                            avatar={entry.avatar}
                                            username={entry.username}
                                        />
                                    </Link>
                                </div>
                                <div className="entry-info">
                                    <div className="entry-username-row">
                                        <div className="entry-username">
                                            <Link className="entry-username-link" to={`/profile/${entry.user_id}`}>
                                                {entry.username}
                                            </Link>
                                        </div>
                                        <div className="entry-rank-name">Score on visible challenges</div>
                                    </div>
                                    <div className="entry-score-bar" aria-hidden="true">
                                        <div
                                            className="score-fill"
                                            style={{
                                                width: `${Math.min(100, (entry.total_score / (leaderValue || 1)) * 100)}%`,
                                            }}
                                        />
                                    </div>
                                </div>
                                <div className="entry-score">
                                    {entry.total_score.toLocaleString()}
                                    <span className="score-label">pts</span>
                                </div>
                            </li>
                        );
                    })}
                </ol>
            )}
            {state.nextCursor && (
                <div className="leaderboard-pagination">
                    <button className="btn btn-secondary" disabled={state.loading} onClick={() => void loadMore()}>
                        {state.loading ? 'Loading…' : 'More players'}
                    </button>
                </div>
            )}
        </section>
    );
}
