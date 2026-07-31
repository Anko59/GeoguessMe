import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import Avatar from '../../components/common/Avatar';
import RankBadge from '../../components/progression/RankBadge';
import type { Profile } from '../../types';
import './ProfilePage.css';

export default function ProfilePage() {
    const [profile, setProfile] = useState<Profile | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    const loadProfile = useCallback(async () => {
        setError('');
        setLoading(true);
        try {
            const response = await api.get<Profile>('/auth/profile');
            setProfile(response.data);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to load your profile.'));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        queueMicrotask(() => void loadProfile());
    }, [loadProfile]);

    if (loading) {
        return (
            <main className="profile-page profile-state" aria-busy="true">
                <div className="loading" role="status">
                    <div className="spinner" />
                    <span>Loading your profile…</span>
                </div>
            </main>
        );
    }

    if (error || !profile) {
        return (
            <main className="profile-page profile-state">
                <div className="profile-error" role="alert">
                    <strong>We couldn’t load your profile</strong>
                    <span>{error || 'Your profile is temporarily unavailable.'}</span>
                    <button className="btn btn-secondary" onClick={() => void loadProfile()}>
                        Retry
                    </button>
                </div>
            </main>
        );
    }

    const { rank, global_rank: globalRank } = profile;
    const nextRankText = rank.next_points
        ? `Next rank at ${rank.next_points.toLocaleString()} pts`
        : 'Highest rank reached';

    return (
        <main className="profile-page">
            <header className="profile-topbar">
                <Link to="/groups" className="profile-back-link">
                    ← Groups
                </Link>
                <Link to="/settings" className="profile-settings-link">
                    Edit profile
                </Link>
            </header>

            <section className="profile-hero" aria-labelledby="profile-title">
                <div className="profile-identity">
                    <Avatar
                        userID={profile.id}
                        avatar={profile.avatar}
                        username={profile.username}
                        className="profile-avatar"
                    />
                    <div>
                        <p className="profile-eyebrow">Your adventurer card</p>
                        <h1 id="profile-title">{profile.username}</h1>
                        <p className="profile-email">{profile.email}</p>
                        <p className="profile-rank-name">
                            <RankBadge rank={rank} />
                            {rank.name}
                        </p>
                    </div>
                </div>
                <RankBadge rank={rank} size="large" alt={`${rank.name} badge`} className="profile-badge" />
            </section>

            <section className="profile-trackers" aria-label="Score trackers">
                <article className="profile-stat-card">
                    <span className="profile-stat-label">Total points</span>
                    <strong>{profile.total_points.toLocaleString()}</strong>
                    <span>Lifetime guess score</span>
                </article>
                <article className="profile-stat-card">
                    <span className="profile-stat-label">Guesses made</span>
                    <strong>{profile.guess_count.toLocaleString()}</strong>
                    <span>Places explored</span>
                </article>
                <article className="profile-stat-card profile-stat-rank">
                    <span className="profile-stat-label">Current rank</span>
                    <strong>#{rank.level}</strong>
                    <span className="profile-stat-rank-name">
                        <RankBadge rank={rank} />
                        {rank.name}
                    </span>
                </article>
                <article className="profile-stat-card profile-stat-global-rank">
                    <span className="profile-stat-label">Global rank</span>
                    {globalRank.rank > 0 ? (
                        <>
                            <strong>#{globalRank.rank}</strong>
                            <span>of {globalRank.total_players.toLocaleString()} players</span>
                        </>
                    ) : (
                        <>
                            <strong>Unranked</strong>
                            <span>Guess a location to enter the ranking</span>
                        </>
                    )}
                </article>
            </section>

            <section className="profile-progress-card" aria-labelledby="progress-title">
                <div className="profile-progress-copy">
                    <p className="profile-eyebrow">Keep climbing</p>
                    <h2 id="progress-title">Progress to the next rank</h2>
                    <p>
                        {rank.points_in_rank.toLocaleString()} of {rank.points_to_next.toLocaleString()} points in this
                        rank.
                    </p>
                    <strong>{nextRankText}</strong>
                </div>
                <div
                    className="profile-progress-ring"
                    role="progressbar"
                    aria-label={`Progress to ${rank.next_points ? 'the next rank' : 'the final rank'}`}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={rank.progress_percent}
                    style={{ '--progress': `${rank.progress_percent}%` } as CSSProperties}
                >
                    <span>{rank.progress_percent}%</span>
                </div>
            </section>
        </main>
    );
}
