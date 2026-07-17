import { useState } from 'react';
import { Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../api';
import { useAuth } from '../context/AuthContext';
import LogoutButton from '../components/LogoutButton';

export default function AccountSettings() {
    const { user, refresh } = useAuth(); const [message, setMessage] = useState(''); const [error, setError] = useState(''); const [password, setPassword] = useState('');
    const resend = async (): Promise<void> => { setError(''); try { const response = await api.post<{ message: string }>('/auth/verify/request'); setMessage(response.data.message); } catch (requestError: unknown) { setError(getAPIErrorMessage(requestError, 'Unable to send verification email')); } };
    const removeAccount = async (): Promise<void> => { if (!window.confirm('Delete your account and gameplay data?')) return; try { await api.delete('/auth/account', { data: { password } }); await refresh(); } catch (requestError: unknown) { setError(getAPIErrorMessage(requestError, 'Unable to delete account')); } };
    return <main className="auth-container"><section className="auth-card"><h1 className="auth-title gradient-text">Account settings</h1><p>Signed in as <strong>{user?.username}</strong></p><p>{user?.email}</p><p>{user?.email_verified_at ? 'Email verified' : 'Email not verified'}</p>{!user?.email_verified_at && <button className="btn btn-secondary" onClick={() => void resend()}>Resend verification email</button>}{message && <p className="auth-success" role="status">{message}</p>}{error && <p className="auth-error" role="alert">{error}</p>}<label htmlFor="delete-password">Confirm password to delete account</label><input id="delete-password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} /><button className="btn btn-outline" onClick={() => void removeAccount()}>Delete account</button><LogoutButton /><Link to="/groups" className="auth-link">Back to groups</Link></section></main>;
}
