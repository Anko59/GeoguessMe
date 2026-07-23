import { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate, useSearchParams, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import Icon from '../../components/ui/Icon';
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
    const [joining, setJoining] = useState(false);
    const autoJoinAttempted = useRef(false);

    const joinGroup = useCallback(async (): Promise<void> => {
        setError('');
        setJoining(true);
        try {
            const res = await api.post('/group/join', { code: code.trim().toUpperCase() });
            navigate(`/group/${res.data.id}`);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Failed to join group'));
        } finally {
            setJoining(false);
        }
    }, [code, navigate]);

    useEffect(() => {
        if (mode === 'join' && initialCode && !autoJoinAttempted.current) {
            autoJoinAttempted.current = true;
            void joinGroup();
        }
    }, [initialCode, joinGroup, mode]);

    const handleJoin = async (e: React.FormEvent): Promise<void> => {
        e.preventDefault();
        await joinGroup();
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
                <Icon name="arrow-left" className="back-arrow-page" />
                <span>Back to Groups</span>
            </Link>

            <div className="group-join-header">
                <img src="/logo.png" alt="" className="join-logo" />
                <p className="join-eyebrow">Play together</p>
                <h1>Find your group</h1>
                <p>Join with a six-character code or start a new circle.</p>
            </div>

            <div className="mode-selector" aria-label="Group action">
                <button
                    aria-pressed={mode === 'join'}
                    onClick={() => setMode('join')}
                    className={`mode-btn ${mode === 'join' ? 'active' : ''}`}
                >
                    Join Group
                </button>
                <button
                    aria-pressed={mode === 'create'}
                    onClick={() => setMode('create')}
                    className={`mode-btn ${mode === 'create' ? 'active' : ''}`}
                >
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
                    <button type="submit" className="btn btn-accent" disabled={joining}>
                        {joining ? 'Joining…' : 'Join Group'}
                    </button>
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
                    <button type="submit" className="btn btn-accent">
                        Create Group
                    </button>
                </form>
            )}

            {error && (
                <div className="error-message" role="alert">
                    {error}
                </div>
            )}
        </div>
    );
}
