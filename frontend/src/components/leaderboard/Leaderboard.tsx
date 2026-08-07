import { useState, useEffect, useCallback, useRef } from 'react';
import { Link } from 'react-router-dom';
import { useAuth } from '../../context/AuthContext';
import type { LeaderboardEntry, LeaderboardMetric, LeaderboardPeriod } from '../../types';
import Avatar from '../common/Avatar';
import RankBadge from '../progression/RankBadge';
import { getCachedLeaderboard, refreshLeaderboard } from './leaderboardCache';
import './Leaderboard.css';

interface LeaderboardProps {
    groupID: string;
}

export default function Leaderboard({ groupID }: LeaderboardProps) {
    const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);
    const [period, setPeriod] = useState<LeaderboardPeriod>('week');
    const [metric, setMetric] = useState<LeaderboardMetric>('total');
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const mountedRef = useRef(true);
    const { user } = useAuth();
    const currentUserId = user?.id;

    const fetchLeaderboard = useCallback(async () => {
        if (!currentUserId) return;
        if (!mountedRef.current) return;
        setError('');
        const cached = getCachedLeaderboard(currentUserId, groupID, period, metric);
        if (cached) {
            setLeaderboard(cached);
            setLoading(false);
        } else {
            setLoading(true);
        }
        try {
            const leaderboard = await refreshLeaderboard(currentUserId, groupID, period, metric);
            if (!mountedRef.current) return;
            setLeaderboard(leaderboard);
        } catch {
            if (!mountedRef.current) return;
            setError('Unable to load rankings. Try again.');
        } finally {
            if (mountedRef.current) setLoading(false);
        }
    }, [currentUserId, groupID, period, metric]);

    useEffect(() => {
        mountedRef.current = true;
        queueMicrotask(() => void fetchLeaderboard());
        const interval = setInterval(() => void fetchLeaderboard(), 10000);
        return () => {
            mountedRef.current = false;
            clearInterval(interval);
        };
    }, [fetchLeaderboard]);

    const getRankEmoji = (rank: number) => {
        switch (rank) {
            case 1:
                return '🥇';
            case 2:
                return '🥈';
            case 3:
                return '🥉';
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

    const periodLabel: Record<LeaderboardPeriod, string> = { week: 'This week', month: 'This month', all: 'All time' };
    const metricLabel: Record<LeaderboardMetric, string> = { total: 'Total', average: 'Average', elo: 'Elo' };

    const metricValue = (entry: LeaderboardEntry): number => {
        switch (metric) {
            case 'average':
                return entry.average_score;
            case 'elo':
                return entry.elo;
            default:
                return entry.score;
        }
    };

    const formatMetric = (value: number): string => {
        if (metric === 'elo' && value === 0) return '—';
        return metric === 'average' ? value.toFixed(1) : Math.round(value).toLocaleString();
    };

    const metricUnit: Record<LeaderboardMetric, string> = { total: 'pts', average: 'avg', elo: 'elo' };

    return (
        <div className="leaderboard-container">
            <div className="leaderboard-header">
                <img src="/friends_leaderboard_icon.png" alt="" className="leaderboard-icon" />
                <div>
                    <p>Group rankings</p>
                    <h2>Leaderboard</h2>
                </div>
            </div>

            <div className="leaderboard-metric-tabs" role="tablist" aria-label="Leaderboard metric">
                {(Object.keys(metricLabel) as LeaderboardMetric[]).map((option) => (
                    <button
                        key={option}
                        type="button"
                        role="tab"
                        aria-selected={metric === option}
                        className={`leaderboard-metric-tab${metric === option ? ' selected' : ''}`}
                        onClick={() => {
                            setMetric(option);
                        }}
                    >
                        {metricLabel[option]}
                    </button>
                ))}
            </div>

            <div className="leaderboard-period-tabs" role="tablist" aria-label="Leaderboard period">
                {(Object.keys(periodLabel) as LeaderboardPeriod[]).map((option) => (
                    <button
                        key={option}
                        type="button"
                        role="tab"
                        aria-selected={period === option}
                        className={`leaderboard-period-tab${period === option ? ' selected' : ''}`}
                        onClick={() => {
                            setPeriod(option);
                        }}
                    >
                        {periodLabel[option]}
                    </button>
                ))}
            </div>

            {loading ? (
                <div className="loading-state">
                    <div className="spinner"></div>
                    <p>Loading {periodLabel[period].toLowerCase()} rankings...</p>
                </div>
            ) : error ? (
                <div className="loading-state" role="alert">
                    <p>{error}</p>
                    <button
                        className="btn btn-secondary"
                        onClick={() => {
                            setLoading(true);
                            void fetchLeaderboard();
                        }}
                    >
                        Retry
                    </button>
                </div>
            ) : leaderboard.length === 0 ? (
                <div className="leaderboard-empty-state">
                    <img src="/cup_icon.png" alt="" className="leaderboard-empty-icon" />
                    <h2>No scores yet</h2>
                    <p className="empty-subtitle">
                        Be the first to guess a location {periodLabel[period].toLowerCase()}.
                    </p>
                </div>
            ) : (
                <div className="leaderboard-list">
                    {leaderboard.map((entry, index) => {
                        const rank = index + 1;
                        const isCurrentUser = entry.user_id === currentUserId;
                        const isUnrated = metric === 'elo' && entry.elo === 0;
                        const rankEmoji = isUnrated ? null : getRankEmoji(rank);
                        const rankClass = isUnrated ? '' : getRankClass(rank);
                        const value = metricValue(entry);
                        const leaderValue = leaderboard[0] ? metricValue(leaderboard[0]) : 1;

                        return (
                            <div
                                key={entry.user_id}
                                className={`leaderboard-entry ${rankClass} ${isCurrentUser ? 'current-user' : ''} scale-in`}
                                style={{ animationDelay: `${index * 0.05}s` }}
                            >
                                <div className="entry-rank">
                                    {isUnrated ? (
                                        '—'
                                    ) : rankEmoji ? (
                                        <img src={rankEmoji} alt="" className="entry-rank-medal" />
                                    ) : (
                                        `#${rank}`
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
                                            {isCurrentUser && <span className="you-badge">You</span>}
                                        </div>
                                        <div className="entry-rank-name">
                                            <RankBadge rank={entry.rank} />
                                            {entry.rank.name}
                                        </div>
                                    </div>
                                    <div className="entry-score-bar">
                                        <div
                                            className="score-fill"
                                            style={{ width: `${Math.min(100, (value / (leaderValue || 1)) * 100)}%` }}
                                        ></div>
                                    </div>
                                </div>

                                <div className="entry-score">
                                    {formatMetric(value)}
                                    <span className="score-label">{metricUnit[metric]}</span>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
}
