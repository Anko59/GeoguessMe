import { useState, useEffect } from 'react';
import api from '../api';
import './Leaderboard.css';

interface LeaderboardEntry {
    user_id: string;
    username: string;
    score: number;
}

interface LeaderboardProps {
    groupID: string;
}

export default function Leaderboard({ groupID }: LeaderboardProps) {
    const [leaderboard, setLeaderboard] = useState<LeaderboardEntry[]>([]);
    const [loading, setLoading] = useState(true);
    const currentUserId = JSON.parse(localStorage.getItem('user') || '{}').id;

    useEffect(() => {
        fetchLeaderboard();
        // Refresh every 10 seconds
        const interval = setInterval(fetchLeaderboard, 10000);
        return () => clearInterval(interval);
    }, [groupID]);

    const fetchLeaderboard = async () => {
        try {
            const res = await api.get(`/group/leaderboard?group_id=${groupID}`);
            setLeaderboard(res.data || []);
        } catch (err) {
            console.error('Failed to fetch leaderboard', err);
        } finally {
            setLoading(false);
        }
    };

    const getRankEmoji = (rank: number) => {
        switch (rank) {
            case 1: return '🥇';
            case 2: return '🥈';
            case 3: return '🥉';
            default: return null;
        }
    };

    const getRankClass = (rank: number) => {
        switch (rank) {
            case 1: return 'gold';
            case 2: return 'silver';
            case 3: return 'bronze';
            default: return '';
        }
    };

    if (loading) {
        return (
            <div className="leaderboard-container">
                <div className="loading-state">
                    <div className="spinner"></div>
                    <p>Loading rankings...</p>
                </div>
            </div>
        );
    }

    if (leaderboard.length === 0) {
        return (
            <div className="leaderboard-container">
                <div className="empty-state">
                    <div className="empty-icon">🏆</div>
                    <p>No scores yet</p>
                    <p className="empty-subtitle">Be the first to guess a location!</p>
                </div>
            </div>
        );
    }

    return (
        <div className="leaderboard-container">
            <div className="leaderboard-header">
                <img src="/friends_leaderboard_icon.png" alt="" className="leaderboard-icon" />
                <h2>Leaderboard</h2>
            </div>

            <div className="leaderboard-list">
                {Array.isArray(leaderboard) && leaderboard.map((entry, index) => {
                    const rank = index + 1;
                    const isCurrentUser = entry.user_id === currentUserId;
                    const rankEmoji = getRankEmoji(rank);
                    const rankClass = getRankClass(rank);

                    return (
                        <div
                            key={entry.user_id}
                            className={`leaderboard-entry ${rankClass} ${isCurrentUser ? 'current-user' : ''} scale-in`}
                            style={{ animationDelay: `${index * 0.05}s` }}
                        >
                            <div className="entry-rank">
                                {rankEmoji || `#${rank}`}
                            </div>

                            <div className="entry-avatar">
                                <img src="/avatar.png" alt={entry.username} />
                            </div>

                            <div className="entry-info">
                                <div className="entry-username">
                                    {entry.username}
                                    {isCurrentUser && <span className="you-badge">You</span>}
                                </div>
                                <div className="entry-score-bar">
                                    <div
                                        className="score-fill"
                                        style={{ width: `${Math.min(100, (entry.score / (leaderboard[0]?.score || 1)) * 100)}%` }}
                                    ></div>
                                </div>
                            </div>

                            <div className="entry-score">
                                {entry.score}
                                <span className="score-label">pts</span>
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
