import { useState, useEffect, useCallback } from 'react';
import { Link } from 'react-router-dom';
import api from '../../api';
import type { Group } from '../../types';
import Icon from '../../components/ui/Icon';
import './GroupsList.css';

export default function GroupsList() {
    const [groups, setGroups] = useState<Group[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    const fetchGroups = useCallback(async () => {
        try {
            const res = await api.get('/user/groups');
            setGroups(res.data || []);
        } catch {
            setError('Unable to load groups. Try again.');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        queueMicrotask(() => void fetchGroups());
    }, [fetchGroups]);

    if (loading) {
        return (
            <div className="groups-list-container">
                <div className="loading" role="status">
                    <div className="spinner" />
                    <span>Loading your groups…</span>
                </div>
            </div>
        );
    }

    if (error)
        return (
            <div className="groups-list-container">
                <div className="groups-state error-message" role="alert">
                    <strong>We couldn’t load your groups</strong>
                    <span>{error}</span>
                    <button
                        className="btn btn-secondary"
                        onClick={() => {
                            setError('');
                            setLoading(true);
                            void fetchGroups();
                        }}
                    >
                        Retry
                    </button>
                </div>
            </div>
        );

    return (
        <div className="groups-list-container">
            <header className="groups-topbar">
                <Link to="/groups" className="groups-brand" aria-label="GeoGuessMe groups">
                    <img src="/logo.png" alt="" />
                    <span>GeoGuessMe</span>
                </Link>
                <Link to="/settings" className="groups-account-link">
                    Account settings
                </Link>
            </header>

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
                        <small>Enter an invite code</small>
                    </span>
                </Link>
            </div>

            {groups.length === 0 ? (
                <div className="empty-state">
                    <img src="/globe_icon.png" alt="" className="empty-icon" />
                    <h2>No groups yet</h2>
                    <p>You haven't joined any groups yet</p>
                    <p className="empty-subtitle">Create or join a group to start playing!</p>
                </div>
            ) : (
                <div className="groups-grid">
                    {groups.map((group) => (
                        <Link key={group.id} to={`/group/${group.id}`} className="group-card">
                            <div className="group-card-content">
                                <img src="/friends_group_icon.png" alt="" className="group-icon" />
                                <div className="group-info">
                                    <h3>{group.name}</h3>
                                    <span className="group-code">#{group.code}</span>
                                </div>
                            </div>
                            <Icon name="chevron-right" className="group-arrow" />
                        </Link>
                    ))}
                </div>
            )}
        </div>
    );
}
