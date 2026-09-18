import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import { getAPIErrorMessage, publicFeedAPI } from '../../api';
import type { PublicFeedLeaderboardEntry } from '../../types';

type LeaderboardState = {
    items: PublicFeedLeaderboardEntry[];
    nextCursor: string;
    error: string;
    loading: boolean;
};

export default function FeedLeaderboard() {
    const [state, setState] = useState<LeaderboardState>({ items: [], nextCursor: '', error: '', loading: true });
    const loadMoreController = useRef<AbortController | null>(null);
    useEffect(() => {
        const controller = new AbortController();
        void publicFeedAPI
            .leaderboard('', controller.signal)
            .then((page) => {
                if (!controller.signal.aborted) {
                    setState({ items: page.items, nextCursor: page.next_cursor, error: '', loading: false });
                }
            })
            .catch((reason: unknown) => {
                if (!controller.signal.aborted) {
                    setState({
                        items: [],
                        nextCursor: '',
                        error: getAPIErrorMessage(reason, 'Unable to load the feed leaderboard.'),
                        loading: false,
                    });
                }
            });
        return () => {
            controller.abort();
            loadMoreController.current?.abort();
        };
    }, []);

    async function loadMore() {
        if (!state.nextCursor || state.loading) return;
        loadMoreController.current?.abort();
        const controller = new AbortController();
        loadMoreController.current = controller;
        setState((current) => ({ ...current, loading: true, error: '' }));
        try {
            const page = await publicFeedAPI.leaderboard(state.nextCursor, controller.signal);
            setState((current) => ({
                items: [...current.items, ...page.items],
                nextCursor: page.next_cursor,
                error: '',
                loading: false,
            }));
        } catch (reason: unknown) {
            if (!controller.signal.aborted) {
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

    return (
        <section className="profile-feed-leaderboard" aria-labelledby="profile-feed-leaderboard-title">
            <div className="profile-feed-leaderboard-heading">
                <div>
                    <p className="profile-eyebrow">Community challenge totals</p>
                    <h2 id="profile-feed-leaderboard-title">Feed leaderboard</h2>
                </div>
                <span>Score only</span>
            </div>
            {state.error && (
                <p className="profile-error" role="alert">
                    {state.error}
                </p>
            )}
            {!state.error && state.loading && state.items.length === 0 && <p role="status">Loading feed rankings…</p>}
            {!state.error && !state.loading && state.items.length === 0 && <p>No feed guesses have been scored yet.</p>}
            {state.items.length > 0 && (
                <ol className="profile-feed-leaderboard-list">
                    {state.items.map((entry) => (
                        <li key={entry.user_id}>
                            <span className="profile-feed-rank">#{entry.rank}</span>
                            <Link className="profile-feed-player" to={`/profile/${entry.user_id}`}>
                                {entry.username}
                            </Link>
                            <strong>{entry.total_score.toLocaleString()}</strong>
                        </li>
                    ))}
                </ol>
            )}
            {state.nextCursor && (
                <button className="btn btn-secondary" disabled={state.loading} onClick={() => void loadMore()}>
                    {state.loading ? 'Loading…' : 'More players'}
                </button>
            )}
        </section>
    );
}
