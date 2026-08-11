import { useCallback, useEffect, useState } from 'react';
import { useNavigate, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import Icon from '../../components/ui/Icon';
import { clearPendingInviteToken, readPendingInviteToken } from '../../hooks/useInviteFragmentCapture';
import type { InvitePreview } from '../../types';
import './GroupJoin.css';

type InviteState = 'checking' | 'ready' | 'missing' | 'invalid' | 'error';

export default function GroupJoin() {
    const navigate = useNavigate();
    const location = useLocation();
    const [mode, setMode] = useState<'join' | 'create'>(location.pathname.endsWith('/create') ? 'create' : 'join');
    const [invite, setInvite] = useState<InvitePreview | null>(null);
    // Derived synchronously from the pending token so the mount state never
    // requires a setState-in-effect round trip (react-hooks lint).
    const [inviteState, setInviteState] = useState<InviteState>(() =>
        readPendingInviteToken() ? 'checking' : 'missing',
    );
    const [inviteError, setInviteError] = useState('');
    const [name, setName] = useState('');
    const [error, setError] = useState('');
    const [joining, setJoining] = useState(false);

    useEffect(() => {
        if (mode !== 'join') return;
        let active = true;
        const token = readPendingInviteToken();
        if (!token) return; // the lazy initializer already rendered the 'missing' state
        void api
            .post<InvitePreview>('/group/invites/preview', { invite_token: token })
            .then((response) => {
                if (!active) return;
                setInvite(response.data);
                setInviteError('');
                setInviteState('ready');
            })
            .catch((requestError: unknown) => {
                if (!active) return;
                if (
                    requestError instanceof Error &&
                    (requestError as { response?: { status?: number } }).response?.status === 404
                ) {
                    // Unknown, expired, and revoked tokens all return a generic 404.
                    clearPendingInviteToken();
                    setInviteState('invalid');
                    return;
                }
                setInviteState('error');
                setInviteError(getAPIErrorMessage(requestError, 'Unable to check this invite link'));
            });
        return () => {
            active = false;
        };
    }, [mode]);

    const joinGroup = useCallback(async (): Promise<void> => {
        setError('');
        setJoining(true);
        try {
            const token = readPendingInviteToken();
            if (!token) {
                setInviteState('missing');
                return;
            }
            const res = await api.post('/group/join', { invite_token: token });
            clearPendingInviteToken();
            navigate(`/group/${res.data.id}`);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Failed to join group'));
        } finally {
            setJoining(false);
        }
    }, [navigate]);

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

    const showJoin = () => {
        if (mode !== 'join') {
            setMode('join');
            // Reset the join state machine for the freshly entered join tab;
            // the effect below re-runs on the mode change and revalidates.
            setInviteState(readPendingInviteToken() ? 'checking' : 'missing');
            setInviteError('');
        }
        setError('');
    };

    const showCreate = () => {
        setMode('create');
        setError('');
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
                <p>Join with an invite link or start a new circle.</p>
            </div>

            <div className="mode-selector" aria-label="Group action">
                <button
                    aria-pressed={mode === 'join'}
                    onClick={showJoin}
                    className={`mode-btn ${mode === 'join' ? 'active' : ''}`}
                >
                    Join Group
                </button>
                <button
                    aria-pressed={mode === 'create'}
                    onClick={showCreate}
                    className={`mode-btn ${mode === 'create' ? 'active' : ''}`}
                >
                    Create Group
                </button>
            </div>

            {mode === 'join' ? (
                <div className="join-form">
                    {inviteState === 'checking' && (
                        <div className="invite-state" role="status">
                            <div className="spinner" />
                            <span>Checking invite…</span>
                        </div>
                    )}
                    {inviteState === 'missing' && (
                        <div className="invite-state" role="alert">
                            <h2>No invite link found</h2>
                            <p>Open the invite link you received, or ask a group member for a new invite.</p>
                        </div>
                    )}
                    {inviteState === 'invalid' && (
                        <div className="invite-state" role="alert">
                            <h2>This invite link is invalid or has expired</h2>
                            <p>Ask a group member for a new invite.</p>
                        </div>
                    )}
                    {inviteState === 'error' && (
                        <div className="invite-state" role="alert">
                            <h2>Unable to check this invite link</h2>
                            <p>{inviteError}</p>
                        </div>
                    )}
                    {inviteState === 'ready' && invite && (
                        <>
                            <h2>Join {invite.group_name}?</h2>
                            <p className="invite-members">
                                {invite.member_count} member{invite.member_count === 1 ? '' : 's'}
                            </p>
                            <button
                                type="button"
                                className="btn btn-accent"
                                disabled={joining}
                                onClick={() => void joinGroup()}
                                data-testid="join-btn"
                            >
                                {joining ? 'Joining…' : 'Join Group'}
                            </button>
                        </>
                    )}
                    {error && (
                        <div className="error-message" role="alert">
                            {error}
                        </div>
                    )}
                </div>
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
                    {error && (
                        <div className="error-message" role="alert">
                            {error}
                        </div>
                    )}
                </form>
            )}
        </div>
    );
}
