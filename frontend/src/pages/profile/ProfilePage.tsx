import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { Link, useParams } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import Avatar from '../../components/common/Avatar';
import { useAvatarUrl } from '../../components/common/avatarCache';
import RankBadge from '../../components/progression/RankBadge';
import FullScreenImage from '../../components/ui/FullScreenImage';
import Icon from '../../components/ui/Icon';
import { useAuth } from '../../context/AuthContext';
import type { Profile, PublicProfile } from '../../types';
import './ProfilePage.css';

export default function ProfilePage() {
    const { userId } = useParams();
    const { user } = useAuth();
    const isOwnProfile = !userId;
    const isSelf = isOwnProfile || user?.id === userId;
    const [profile, setProfile] = useState<Profile | PublicProfile | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    // Resolved once per profile so the hero avatar can open full screen; the
    // hook is called unconditionally to keep the hook order stable across the
    // loading/error early returns.
    const avatarURL = useAvatarUrl(profile?.id ?? '', profile?.avatar);

    const loadProfile = useCallback(async () => {
        setError('');
        setLoading(true);
        try {
            if (isOwnProfile) {
                const response = await api.get<Profile>('/auth/profile');
                setProfile(response.data);
            } else {
                const response = await api.get<PublicProfile>(`/user/profile/${userId}`);
                setProfile(response.data);
            }
        } catch (requestError: unknown) {
            setError(
                getAPIErrorMessage(
                    requestError,
                    isOwnProfile ? 'Unable to load your profile.' : "Unable to load this player's profile.",
                ),
            );
        } finally {
            setLoading(false);
        }
    }, [isOwnProfile, userId]);

    useEffect(() => {
        const task = window.setTimeout(() => void loadProfile(), 0);
        return () => window.clearTimeout(task);
    }, [loadProfile]);

    if (loading) {
        return (
            <main className="profile-page profile-state" aria-busy="true">
                <div className="loading" role="status">
                    <div className="spinner" />
                    <span>Loading profile…</span>
                </div>
            </main>
        );
    }

    if (error || !profile) {
        return (
            <main className="profile-page profile-state">
                <div className="profile-error" role="alert">
                    <strong>We couldn’t load this profile</strong>
                    <span>{error || 'This profile is temporarily unavailable.'}</span>
                    <button className="btn btn-secondary" onClick={() => void loadProfile()}>
                        Retry
                    </button>
                </div>
            </main>
        );
    }

    const { rank } = profile;
    const remaining = rank.next_points ? rank.points_to_next - rank.points_in_rank : 0;

    return (
        <main className="profile-page">
            <header className="profile-topbar">
                <Link to="/groups" className="profile-back-link">
                    <Icon name="arrow-left" className="profile-back-icon" />
                    Groups
                </Link>
                {isSelf && (
                    <Link to="/settings" className="profile-settings-link">
                        Settings
                    </Link>
                )}
            </header>

            <section className="profile-hero" aria-labelledby="profile-title">
                <div className="profile-identity">
                    <div className="profile-avatar-ring">
                        <FullScreenImage src={avatarURL} alt={`${profile.username}'s avatar`}>
                            <Avatar
                                userID={profile.id}
                                avatar={profile.avatar}
                                username={profile.username}
                                className="profile-avatar"
                            />
                        </FullScreenImage>
                    </div>
                    <div>
                        <p className="profile-eyebrow">Adventurer card</p>
                        <h1 id="profile-title">{profile.username}</h1>
                        {isOwnProfile && 'email' in profile && profile.email_verified_at && profile.email && (
                            <p className="profile-email">{profile.email}</p>
                        )}
                        <p className="profile-rank-name">
                            <RankBadge rank={rank} />
                            {rank.name}
                        </p>
                    </div>
                </div>
                <RankBadge rank={rank} size="large" alt={`${rank.name} badge`} className="profile-badge" />
            </section>

            <section className="profile-trackers" aria-label="Score trackers">
                <article className="profile-stat-card profile-stat-points">
                    <span className="profile-stat-label">Total points</span>
                    <strong>{profile.total_points.toLocaleString()}</strong>
                    {profile.global_rank.rank > 0 ? (
                        <span>
                            #{profile.global_rank.rank} of {profile.global_rank.total_players.toLocaleString()} players
                        </span>
                    ) : (
                        <span>Guess a location to enter the ranking</span>
                    )}
                </article>
                <article className="profile-stat-card profile-stat-guesses">
                    <span className="profile-stat-label">Guesses made</span>
                    <strong>{profile.guess_count.toLocaleString()}</strong>
                    <span>Places explored</span>
                </article>
                <article className="profile-stat-card profile-stat-average">
                    <span className="profile-stat-label">Average score</span>
                    <strong>{profile.average_score.toFixed(1)}</strong>
                    {profile.global_average_rank.rank > 0 ? (
                        <span>
                            #{profile.global_average_rank.rank} of{' '}
                            {profile.global_average_rank.total_players.toLocaleString()} players
                        </span>
                    ) : (
                        <span>Guess a location to enter the ranking</span>
                    )}
                </article>
                <article className="profile-stat-card profile-stat-elo">
                    <span className="profile-stat-label">Elo rating</span>
                    <strong>{profile.elo > 0 ? profile.elo.toLocaleString() : '—'}</strong>
                    {profile.global_elo_rank.rank > 0 ? (
                        <span>
                            #{profile.global_elo_rank.rank} of {profile.global_elo_rank.total_players.toLocaleString()}{' '}
                            rated players
                        </span>
                    ) : (
                        <span>Guess a shared challenge to get rated</span>
                    )}
                </article>
                <article className="profile-stat-card profile-stat-rank">
                    <span className="profile-stat-label">Current rank</span>
                    <strong>#{rank.level}</strong>
                    <span className="profile-stat-rank-name">
                        <RankBadge rank={rank} />
                        {rank.name}
                    </span>
                </article>
            </section>

            <section className="profile-next-rank" aria-labelledby="next-rank-title">
                <div className="profile-next-rank-copy">
                    <p className="profile-eyebrow">Keep climbing</p>
                    <h2 id="next-rank-title">
                        {rank.next_rank ? `Next rank: ${rank.next_rank.name}` : 'Highest rank reached'}
                    </h2>
                    {rank.next_rank ? (
                        <>
                            <div className="profile-rank-path" aria-hidden="true">
                                <RankBadge rank={rank} />
                                <span className="profile-rank-arrow">
                                    <Icon name="chevron-right" />
                                </span>
                                <RankBadge rank={rank.next_rank} />
                            </div>
                            <p className="profile-next-rank-progress">
                                {rank.points_in_rank.toLocaleString()} of {rank.points_to_next.toLocaleString()} points
                                — {remaining.toLocaleString()} to go
                            </p>
                        </>
                    ) : (
                        <p className="profile-next-rank-progress">
                            You’ve reached the top of the ladder.{' '}
                            <img src="/ui/crown.png" alt="" className="profile-crown-icon" />
                        </p>
                    )}
                </div>
                <div
                    className="profile-progress-ring"
                    role="progressbar"
                    aria-label={`Progress to ${rank.next_rank ? 'the next rank' : 'the final rank'}`}
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
