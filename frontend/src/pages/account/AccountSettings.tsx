import { useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import LogoutButton from '../../components/navigation/LogoutButton';
import './AccountSettings.css';

const avatars = Array.from({ length: 10 }, (_, index) => (index === 0 ? 'avatar.png' : `avatar${index + 1}.png`));

export default function AccountSettings() {
    const { user, refresh, logout } = useAuth();
    const navigate = useNavigate();
    const [username, setUsername] = useState(user?.username ?? '');
    const [email, setEmail] = useState(user?.email ?? '');
    const [avatar, setAvatar] = useState(user?.avatar ?? 'avatar.png');
    const [profilePassword, setProfilePassword] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [password, setPassword] = useState('');
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);

    const clearNotice = () => {
        setMessage('');
        setError('');
    };

    const saveProfile = async (): Promise<void> => {
        clearNotice();
        setSaving(true);
        try {
            await api.patch('/auth/profile', { username, email, avatar, current_password: profilePassword });
            setProfilePassword('');
            await refresh();
            setMessage('Profile updated.');
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to update profile'));
        } finally {
            setSaving(false);
        }
    };

    const changePassword = async (): Promise<void> => {
        clearNotice();
        setSaving(true);
        try {
            await api.post('/auth/password/change', { current_password: profilePassword, new_password: newPassword });
            await logout();
            navigate('/login', { replace: true });
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to change password'));
        } finally {
            setSaving(false);
        }
    };

    const resend = async (): Promise<void> => {
        clearNotice();
        try {
            const response = await api.post<{ message: string }>('/auth/verify/request');
            setMessage(response.data.message);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to send verification email'));
        }
    };

    const removeAccount = async (): Promise<void> => {
        if (!window.confirm('Delete your account and gameplay data?')) return;
        clearNotice();
        try {
            await api.delete('/auth/account', { data: { password } });
            await refresh();
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to delete account'));
        }
    };

    return (
        <main className="auth-container account-settings-container">
            <section className="auth-card account-settings-card">
                <div className="account-header">
                    <img src="/logo.png" alt="" />
                    <div>
                        <p className="account-eyebrow">Your GeoGuessMe profile</p>
                        <h1 className="auth-title">Account settings</h1>
                        <p className="account-intro">Manage your profile, security, and account.</p>
                    </div>
                </div>

                <div className="account-section">
                    <div className="account-section-heading">
                        <h2>Profile</h2>
                        <p>How friends see you in groups and results.</p>
                    </div>
                    <div className="avatar-picker" role="radiogroup" aria-label="Profile image">
                        {avatars.map((candidate) => (
                            <button
                                key={candidate}
                                type="button"
                                className={`avatar-choice${avatar === candidate ? ' selected' : ''}`}
                                aria-label={`Choose ${candidate}`}
                                aria-pressed={avatar === candidate}
                                onClick={() => setAvatar(candidate)}
                            >
                                <img src={`/avatars/${candidate}`} alt="" />
                            </button>
                        ))}
                    </div>
                    <label htmlFor="settings-username">Username</label>
                    <input
                        id="settings-username"
                        value={username}
                        onChange={(event) => setUsername(event.target.value)}
                    />
                    <label htmlFor="settings-email">Email address</label>
                    <input
                        id="settings-email"
                        type="email"
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                    />
                    <label htmlFor="profile-current-password">Current password to save profile changes</label>
                    <input
                        id="profile-current-password"
                        type="password"
                        autoComplete="current-password"
                        value={profilePassword}
                        onChange={(event) => setProfilePassword(event.target.value)}
                    />
                    <button className="btn btn-primary" disabled={saving} onClick={() => void saveProfile()}>
                        Save profile
                    </button>
                </div>

                <div className="account-section">
                    <div className="account-section-heading">
                        <h2>Security</h2>
                        <p>Choose a strong password you do not use elsewhere.</p>
                    </div>
                    <label htmlFor="new-password">New password</label>
                    <input
                        id="new-password"
                        type="password"
                        autoComplete="new-password"
                        value={newPassword}
                        onChange={(event) => setNewPassword(event.target.value)}
                    />
                    <p className="account-help">Use at least 8 characters with uppercase, lowercase, and a number.</p>
                    <button className="btn btn-secondary" disabled={saving} onClick={() => void changePassword()}>
                        Change password
                    </button>
                </div>

                {message && (
                    <p className="auth-success" role="status">
                        {message}
                    </p>
                )}
                {error && (
                    <p className="auth-error" role="alert">
                        {error}
                    </p>
                )}
                <div className="account-verification">
                    <div>
                        <strong>{user?.email_verified_at ? 'Email verified' : 'Email not verified'}</strong>
                        <span>
                            {user?.email_verified_at
                                ? 'Your account recovery address is confirmed.'
                                : 'Verify your address to secure account recovery.'}
                        </span>
                    </div>
                    {!user?.email_verified_at && (
                        <button className="btn btn-secondary" onClick={() => void resend()}>
                            Resend verification email
                        </button>
                    )}
                </div>
                <div className="account-danger">
                    <div className="account-section-heading">
                        <h2>Danger zone</h2>
                        <p>Permanently delete your account and gameplay data.</p>
                    </div>
                    <label htmlFor="delete-password">Confirm password to delete account</label>
                    <input
                        id="delete-password"
                        type="password"
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                    />
                    <button className="btn btn-danger" onClick={() => void removeAccount()}>
                        Delete account
                    </button>
                </div>
                <div className="account-footer-actions">
                    <LogoutButton />
                    <Link to="/groups" className="auth-link">
                        Back to groups
                    </Link>
                </div>
            </section>
        </main>
    );
}
