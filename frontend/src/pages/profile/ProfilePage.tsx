import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react';
import { useParams } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import Avatar from '../../components/common/Avatar';
import { useAvatarUrl } from '../../components/common/avatarCache';
import RankBadge from '../../components/progression/RankBadge';
import AuthenticatedPageShell from '../../components/layout/AuthenticatedPageShell';
import FullScreenImage from '../../components/ui/FullScreenImage';
import Icon from '../../components/ui/Icon';
import ReportAction from '../../components/ui/ReportAction';
import { useAuth } from '../../context/AuthContext';
import type { Profile, PublicProfile } from '../../types';
import FeedLeaderboard from './FeedLeaderboard';
import MapPinProfileCard from '../../components/profile/MapPinProfileCard';
import { useUserBlocks } from '../../hooks/useUserBlocks';
import './ProfilePage.css';

export default function ProfilePage() {
    const { userId } = useParams();
    const { user } = useAuth();
    const isOwnProfile = !userId;
    const isSelf = isOwnProfile || user?.id === userId;
    const [profile, setProfile] = useState<Profile | PublicProfile | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const blocks = useUserBlocks(!isSelf);
    const [blockOverride, setBlockOverride] = useState<{ id: string; blocked: boolean } | null>(null);
    const blocked =
        blockOverride && blockOverride.id === userId
            ? blockOverride.blocked
            : blocks.items.some((item) => item.user_id === userId);
    // Resolved once per profile so the hero avatar can open full screen; the
    // hook is called unconditionally to keep the hook order stable across the
    // loading/error early returns.
    const avatarURL = useAvatarUrl(profile?.id ?? '', profile?.avatar);

    const profileRequest = useRef<AbortController | null>(null);
    const loadProfile = useCallback(async () => {
        profileRequest.current?.abort();
        const request = new AbortController();
        profileRequest.current = request;
        setError('');
        setLoading(true);
        try {
            if (isOwnProfile) {
                const response = await api.get<Profile>('/auth/profile', { signal: request.signal });
                if (!request.signal.aborted) setProfile(response.data);
            } else {
                const response = await api.get<PublicProfile>(`/user/profile/${userId}`, { signal: request.signal });
                if (!request.signal.aborted) setProfile(response.data);
            }
        } catch (requestError: unknown) {
            if (request.signal.aborted) return;
            setError(
                getAPIErrorMessage(
                    requestError,
                    isOwnProfile ? 'Unable to load your profile.' : "Unable to load this player's profile.",
                ),
            );
        } finally {
            if (!request.signal.aborted) setLoading(false);
        }
    }, [isOwnProfile, userId]);

    useEffect(() => {
        const task = window.setTimeout(() => void loadProfile(), 0);
        return () => {
            window.clearTimeout(task);
            profileRequest.current?.abort();
        };
    }, [loadProfile]);

    const blockControls =
        !isSelf && userId ? (
            <section aria-label="Player blocking">
                <p>
                    Blocking hides chat, feed, profiles, and media in both directions. Group membership and rankings
                    stay unchanged.
                </p>
                <button
                    className="btn btn-secondary"
                    disabled={blocks.loading || Boolean(blocks.pending) || Boolean(blocks.error)}
                    onClick={() => {
                        if (
                            !blocked &&
                            !window.confirm(
                                'Block this player? Content and interactions will be hidden in both directions.',
                            )
                        )
                            return;
                        void blocks.change(userId, !blocked).then((success) => {
                            if (!success) return;
                            setBlockOverride({ id: userId, blocked: !blocked });
                            if (blocked) void loadProfile();
                        });
                    }}
                >
                    {blocks.pending ? 'Saving…' : blocked ? 'Unblock player' : 'Block player'}
                </button>
                {blocks.error && (
                    <>
                        <p role="alert">{blocks.error}</p>
                        <button className="btn btn-secondary" onClick={blocks.retry}>
                            Retry blocked users
                        </button>
                    </>
                )}
            </section>
        ) : null;

    if (loading) {
        return (
            <AuthenticatedPageShell
                className="profile-page-shell"
                contentClassName="profile-page profile-state"
                contentAs="main"
                showSettings={isSelf}
                ariaBusy
            >
                <div className="loading" role="status">
                    <div className="spinner" />
                    <span>Loading profile…</span>
                </div>
            </AuthenticatedPageShell>
        );
    }

    if (blocked || error || !profile) {
        return (
            <AuthenticatedPageShell
                className="profile-page-shell"
                contentClassName="profile-page profile-state"
                contentAs="main"
                showSettings={isSelf}
            >
                <div className="profile-error" role={blocked ? 'status' : 'alert'}>
                    <strong>{blocked ? 'Player blocked' : 'We couldn’t load this profile'}</strong>
                    <span>
                        {blocked
                            ? 'Unblock this player to restore access, unless they have also blocked you.'
                            : error || 'This profile is temporarily unavailable.'}
                    </span>
                    {!blocked && (
                        <button className="btn btn-secondary" onClick={() => void loadProfile()}>
                            Retry
                        </button>
                    )}
                </div>
                {blockControls}
            </AuthenticatedPageShell>
        );
    }

    const { rank } = profile;
    const remaining = rank.next_points ? rank.points_to_next - rank.points_in_rank : 0;

    return (
        <AuthenticatedPageShell
            className="profile-page-shell"
            contentClassName="profile-page"
            contentAs="main"
            showSettings={isSelf}
        >
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
                            <span className="profile-rank-label">{rank.name}</span>
                        </p>
                    </div>
                </div>
                <RankBadge rank={rank} size="large" alt={`${rank.name} badge`} className="profile-badge" />
            </section>

            {blockControls}
            {!isSelf && <ReportAction key={profile.id} kind="users" targetID={profile.id} />}
            <MapPinProfileCard username={profile.username} pin={profile.map_pin} ownProfile={isSelf} />

            <section className="profile-trackers" aria-label="Score trackers">
                <article className="profile-stat-card profile-stat-points">
                    <span className="profile-stat-label">Total points</span>
                    <strong>{profile.total_points.toLocaleString()}</strong>
                    {profile.global_rank.rank > 0 ? (
                        <span>
                            #{profile.global_rank.rank} of {profile.global_rank.total_players.toLocaleString()} players
                        </span>
                    ) : (
                        <span>Guess a group challenge to enter the ranking</span>
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
                        <span>Guess a group challenge to enter the ranking</span>
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
                        <span className="profile-rank-label">{rank.name}</span>
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
            <FeedLeaderboard profileID={profile.id} profileUsername={profile.username} />
        </AuthenticatedPageShell>
    );
}
