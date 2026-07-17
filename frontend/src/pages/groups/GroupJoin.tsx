import { useState } from 'react';
import { useNavigate, useSearchParams, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import './GroupJoin.css';

export default function GroupJoin() {
    const [searchParams] = useSearchParams();
    const navigate = useNavigate();
    const location = useLocation();
    const initialCode = searchParams.get('code') ?? '';
    const [mode, setMode] = useState<'join' | 'create'>(location.pathname.endsWith('/create') ? 'create' : 'join');
    const [code, setCode] = useState(initialCode);
    const [name, setName] = useState('');
    const [error, setError] = useState('');

    const handleJoin = async (e: React.FormEvent): Promise<void> => {
        e.preventDefault();
        setError('');
        try {
            const res = await api.post('/group/join', { code });
            navigate(`/group/${res.data.id}`);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Failed to join group'));
        }
    };

    const handleCreate = async (e: React.FormEvent): Promise<void> => {
        e.preventDefault();
        setError('');
        try {
            const res = await api.post('/group/create', { name });
            navigate(`/group/${res.data.id}`);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Failed to create group'));
        }
    };

    return (
        <div className="group-join-container">
            <Link to="/groups" className="back-btn-page">
                <img src="/back_arrow.png" alt="Back" className="back-arrow-page" />
                <span>Back to Groups</span>
            </Link>

            <div className="group-join-header">
                <img src="/logo.png" alt="Logo" className="join-logo" />
                <h1 className="gradient-text">Join or Create a Group</h1>
            </div>

            <div className="mode-selector">
                <button onClick={() => setMode('join')} className={`mode-btn ${mode === 'join' ? 'active' : ''}`}>
                    Join Group
                </button>
                <button onClick={() => setMode('create')} className={`mode-btn ${mode === 'create' ? 'active' : ''}`}>
                    Create Group
                </button>
            </div>

            {mode === 'join' ? (
                <form onSubmit={handleJoin} className="join-form">
                    <h2>Enter Group Code</h2>
                    <input
                        type="text"
                        placeholder="6-character code"
                        value={code}
                        onChange={(e) => setCode(e.target.value.toUpperCase())}
                        maxLength={6}
                        required
                    />
                    <button type="submit">Join Group</button>
                </form>
            ) : (
                <form onSubmit={handleCreate} className="join-form">
                    <h2>Name Your Group</h2>
                    <input
                        type="text"
                        placeholder="Group Name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        required
                    />
                    <button type="submit">Create Group</button>
                </form>
            )}

            {error && <div className="error-message">{error}</div>}
        </div>
    );
}
