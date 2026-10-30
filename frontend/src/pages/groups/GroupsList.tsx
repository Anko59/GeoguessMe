import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { getAPIErrorMessage, groupsAPI } from '../../api';
import type { GroupInbox } from '../../types';
import Icon from '../../components/ui/Icon';
import AuthenticatedPageShell from '../../components/layout/AuthenticatedPageShell';
import { useGroupPhotoUrl } from './groupPhotoCache';
import './GroupsList.css';

function GroupCard({ group }: { group: GroupInbox }) {
    const groupPhotoURL = useGroupPhotoUrl(group.id);

    return (
        <Link key={group.id} to={`/group/${group.id}`} className="group-card" data-group-id={group.id}>
            <div className="group-card-content">
                <img src={groupPhotoURL} alt="" className="group-icon" />
                <div className="group-info">
                    <h3>{group.name}</h3>
                </div>
            </div>
            {group.unread_count > 0 && (
                <span
                    className="group-unread-badge"
                    role="status"
                    aria-label={`${group.unread_count} unread ${group.unread_count === 1 ? 'message' : 'messages'}`}
                >
                    {group.unread_count > 99 ? '99+' : group.unread_count}
                </span>
            )}
            <Icon name="chevron-right" className="group-arrow" />
        </Link>
    );
}

export default function GroupsList() {
    const [groups, setGroups] = useState<GroupInbox[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [attempt, setAttempt] = useState(0);

    useEffect(() => {
        let active = true;
        let pending = false;
        let loaded = false;
        const controller = new AbortController();
        const fetchGroups = async () => {
            if (pending || document.visibilityState !== 'visible') return;
            pending = true;
            try {
                const items = await groupsAPI.inbox(controller.signal);
                if (active) {
                    loaded = true;
                    setGroups(items ?? []);
                    setError('');
                }
            } catch (requestError: unknown) {
                if (active) {
                    setError(
                        getAPIErrorMessage(
                            requestError,
                            loaded
                                ? 'Unable to refresh unread counts. Try again.'
                                : 'Unable to load groups. Try again.',
                        ),
                    );
                }
            } finally {
                pending = false;
                if (active) setLoading(false);
            }
        };
        const refresh = () => void fetchGroups();
        refresh();
        const timer = window.setInterval(refresh, 15000);
        window.addEventListener('focus', refresh);
        document.addEventListener('visibilitychange', refresh);
        return () => {
            active = false;
            controller.abort();
            window.clearInterval(timer);
            window.removeEventListener('focus', refresh);
            document.removeEventListener('visibilitychange', refresh);
        };
    }, [attempt]);

    const retry = () => {
        setError('');
        setLoading(true);
        setAttempt((value) => value + 1);
    };

    if (loading) {
        return (
            <AuthenticatedPageShell className="groups-page-shell" contentClassName="groups-list-container">
                <div className="loading" role="status">
                    <div className="spinner" />
                    <span>Loading your groups…</span>
                </div>
            </AuthenticatedPageShell>
        );
    }

    if (error && groups.length === 0)
        return (
            <AuthenticatedPageShell className="groups-page-shell" contentClassName="groups-list-container">
                <div className="groups-state error-message" role="alert">
                    <strong>We couldn’t load your groups</strong>
                    <span>{error}</span>
                    <button className="btn btn-secondary" onClick={retry}>
                        Retry
                    </button>
                </div>
            </AuthenticatedPageShell>
        );

    return (
        <AuthenticatedPageShell className="groups-page-shell" contentClassName="groups-list-container">
            {error && (
                <div className="groups-refresh-error" role="status">
                    <span>{error}</span>
                    <button className="btn btn-secondary" onClick={retry}>
                        Retry
                    </button>
                </div>
            )}
            <div className="groups-heading-row">
                <div className="groups-header">
                    <p className="groups-eyebrow">Your game circles</p>
                    <h1>My Groups</h1>
                    <p className="subtitle">Choose a group or invite friends to start exploring.</p>
                </div>
                <img src="/welcome_banner.png" alt="" className="groups-heading-art" />
            </div>

            <div className="groups-actions">
                <Link to="/group/create" className="action-btn action-create">
                    <img src="/friends_group_icon.png" alt="" className="btn-icon" />
                    <span>
                        <strong>Create Group</strong>
                        <small>Start a new circle</small>
                    </span>
                </Link>
                <Link to="/group/join" className="action-btn action-join">
                    <img src="/join_group_icon.png" alt="" className="btn-icon" />
                    <span>
                        <strong>Join Group</strong>
                        <small>Use an invite link</small>
                    </span>
                </Link>
            </div>

            {groups.length === 0 ? (
                <div className="empty-state">
                    <img src="/globe_icon.png" alt="" className="empty-icon" />
                    <h2>No groups yet</h2>
                    <p>You haven't joined any groups yet</p>
                    <p className="empty-subtitle">Create or join a group to play with friends.</p>
                    <Link to="/feed" className="btn btn-secondary">
                        Play public challenges
                    </Link>
                </div>
            ) : (
                <div className="groups-grid">
                    {groups.map((group) => (
                        <GroupCard key={group.id} group={group} />
                    ))}
                </div>
            )}
        </AuthenticatedPageShell>
    );
}
