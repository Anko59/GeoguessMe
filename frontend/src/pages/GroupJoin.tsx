import { useState, useEffect } from 'react';
import { useNavigate, useSearchParams, Link } from 'react-router-dom';
import api from '../api';

export default function GroupJoin() {
    const [searchParams] = useSearchParams();
    const navigate = useNavigate();
    const [mode, setMode] = useState<'join' | 'create'>('join');
    const [code, setCode] = useState('');
    const [name, setName] = useState('');
    const [error, setError] = useState('');

    // Auto-fill code from URL if present
    useEffect(() => {
        const codeFromUrl = searchParams.get('code');
        if (codeFromUrl) {
            setCode(codeFromUrl);
        }
    }, [searchParams]);

    const handleJoin = async (e: React.FormEvent) => {
        e.preventDefault();
        setError('');
        try {
            const res = await api.post('/group/join', { code });
            navigate(`/ group / ${res.data.id} `);
        } catch (err: any) {
            setError(err.response?.data || 'Failed to join group');
        }
    };

    const handleCreate = async (e: React.FormEvent) => {
        e.preventDefault();
        setError('');
        try {
            const res = await api.post('/group/create', { name });
            navigate(`/ group / ${res.data.id} `);
        } catch (err: any) {
            setError(err.response?.data || 'Failed to create group');
        }
    };

    return (
        <div className="group-join-container">
            <Link to="/groups" className="back-btn-page">← Back to Groups</Link>

            <div className="group-join-header">
                <img src="/logo.png" alt="Logo" className="join-logo" />
                <h1 className="gradient-text">Join or Create a Group</h1>
            </div>

            <div style={{ display: 'flex', marginBottom: '2rem', background: 'var(--input-bg)', borderRadius: '0.5rem', padding: '0.25rem' }}>
                <button
                    onClick={() => setMode('join')}
                    style={{
                        flex: 1,
                        padding: '0.75rem',
                        borderRadius: '0.25rem',
                        background: mode === 'join' ? 'var(--primary-color)' : 'transparent',
                        color: mode === 'join' ? 'white' : 'var(--secondary-color)',
                        fontWeight: 'bold'
                    }}
                >
                    Join Group
                </button>
                <button
                    onClick={() => setMode('create')}
                    style={{
                        flex: 1,
                        padding: '0.75rem',
                        borderRadius: '0.25rem',
                        background: mode === 'create' ? 'var(--primary-color)' : 'transparent',
                        color: mode === 'create' ? 'white' : 'var(--secondary-color)',
                        fontWeight: 'bold'
                    }}
                >
                    Create Group
                </button>
            </div>

            {mode === 'join' ? (
                <form onSubmit={handleJoin} style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                    <h2 style={{ textAlign: 'center' }}>Enter Code</h2>
                    <input
                        type="text"
                        placeholder="6-character code"
                        value={code}
                        onChange={(e) => setCode(e.target.value.toUpperCase())}
                        maxLength={6}
                        required
                    />
                    <button type="submit" className="btn btn-primary">Join</button>
                </form>
            ) : (
                <form onSubmit={handleCreate} style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
                    <h2 style={{ textAlign: 'center' }}>Name Your Group</h2>
                    <input
                        type="text"
                        placeholder="Group Name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        required
                    />
                    <button type="submit" className="btn btn-primary">Create</button>
                </form>
            )}

            {error && <div style={{ color: 'var(--accent-color)', textAlign: 'center', marginTop: '1rem' }}>{error}</div>}
        </div>
    );
}
