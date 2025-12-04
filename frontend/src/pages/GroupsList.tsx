import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import api from '../api';
import './GroupsList.css';

interface Group {
    id: string;
    name: string;
    code: string;
}

export default function GroupsList() {
    const [groups, setGroups] = useState<Group[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        fetchGroups();
    }, []);

    const fetchGroups = async () => {
        try {
            const res = await api.get('/user/groups');
            setGroups(res.data || []);
        } catch (err) {
            console.error('Failed to fetch groups', err);
        } finally {
            setLoading(false);
        }
    };

    if (loading) {
        return (
            <div className="groups-list-container">
                <div className="loading">Loading your groups...</div>
            </div>
        );
    }

    return (
        <div className="groups-list-container">
            <div className="welcome-banner">
                <img src="/welcome_banner.png" alt="Welcome" />
            </div>

            <div className="groups-header">
                <h1 className="gradient-text">My Groups</h1>
                <p className="subtitle">Choose a group to start playing</p>
            </div>

            <div className="groups-actions">
                <Link to="/group/create" className="action-btn btn-primary">
                    <img src="/friends_group_icon.png" alt="" className="btn-icon" />
                    <span>Create Group</span>
                </Link>
                <Link to="/group/join" className="action-btn btn-secondary">
                    <img src="/join_group_icon.png" alt="" className="btn-icon" />
                    <span>Join Group</span>
                </Link>
            </div>

            {groups.length === 0 ? (
                <div className="empty-state">
                    <div className="empty-icon">🌍</div>
                    <p>You haven't joined any groups yet</p>
                    <p className="empty-subtitle">Create or join a group to start playing!</p>
                </div>
            ) : (
                <div className="groups-grid">
                    {groups.map((group) => (
                        <Link
                            key={group.id}
                            to={`/group/${group.id}`}
                            className="group-card"
                        >
                            <div className="group-card-content">
                                <img src="/friends_group_icon.png" alt="" className="group-icon" />
                                <div className="group-info">
                                    <h3>{group.name}</h3>
                                    <span className="group-code">#{group.code}</span>
                                </div>
                            </div>
                            <img src="/foward_arrow_icon.png" alt="" className="group-arrow" />
                        </Link>
                    ))}
                </div>
            )}
        </div>
    );
}
