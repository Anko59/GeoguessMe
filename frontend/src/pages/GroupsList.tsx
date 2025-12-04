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
            <div className="groups-header">
                <h1 className="gradient-text">My Groups</h1>
                <p className="subtitle">Select a group to start guessing</p>
            </div>

            <div className="groups-actions">
                <Link to="/group/create" className="btn btn-primary">
                    + Create Group
                </Link>
                <Link to="/group/join" className="btn btn-secondary">
                    Join Group
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
                            <div className="group-card-header">
                                <h3>{group.name}</h3>
                                <span className="group-code">#{group.code}</span>
                            </div>
                            <div className="group-card-footer">
                                <span className="open-btn">Open ➡</span>
                            </div>
                        </Link>
                    ))}
                </div>
            )}
        </div>
    );
}
