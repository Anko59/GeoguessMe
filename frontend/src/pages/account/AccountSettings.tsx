import { useEffect, useRef, useState, type ChangeEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import Avatar from '../../components/common/Avatar';
import { bustAvatarCache } from '../../components/common/avatarCache';
import LogoutButton from '../../components/navigation/LogoutButton';
import type { OIDCConfig } from '../../types';
import './AccountSettings.css';

const avatars = Array.from({ length: 10 }, (_, index) => (index === 0 ? 'avatar.png' : `avatar${index + 1}.png`));

export default function AccountSettings() {
    const { user, refresh, logout } = useAuth();
    const navigate = useNavigate();
    const [username, setUsername] = useState(user?.username ?? '');
    const [email, setEmail] = useState(user?.pending_email ?? user?.email ?? '');
    const [avatar, setAvatar] = useState(user?.avatar ?? 'avatar.png');
    const [avatarVersion, setAvatarVersion] = useState(0);
    const [uploading, setUploading] = useState(false);
    const [avatarError, setAvatarError] = useState('');
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [profilePassword, setProfilePassword] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [password, setPassword] = useState('');
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);
    const [linking, setLinking] = useState(false);
    const [keycloakAccountURL, setKeycloakAccountURL] = useState('');
    const migrationRequired = Boolean(user?.migration_required);

    useEffect(() => {
        let active = true;
        if (!user?.oidc_linked) return () => undefined;
        void api
            .get<OIDCConfig>('/auth/oidc/config')
            .then((response) => {
                if (active) setKeycloakAccountURL(response.data.account_url ?? '');
            })
            .catch(() => undefined);
        return () => {
            active = false;
        };
    }, [user?.oidc_linked]);

    const clearNotice = () => {
        setMessage('');
        setError('');
        setAvatarError('');
    };

    const uploadPhoto = async (event: ChangeEvent<HTMLInputElement>): Promise<void> => {
        const file = event.target.files?.[0];
        if (!file) return;
        clearNotice();
        setUploading(true);
        try {
            const formData = new FormData();
            // Keep this identical to the working group-photo upload path.
            // The browser may provide a camera photo with an unusual MIME
            // type or filename; the server validates the actual image bytes.
            formData.append('photo', file);
            // Let XMLHttpRequest add the multipart boundary. Setting the
            // content type manually omits that boundary in some browsers and
            // makes the server reject an otherwise valid form.
            await api.post('/auth/profile/avatar', formData);
            if (user) bustAvatarCache(user.id);
            setAvatar('custom');
            setAvatarVersion((value) => value + 1);
            await refresh();
            setMessage('Profile photo updated.');
        } catch (requestError: unknown) {
            setAvatarError(getAPIErrorMessage(requestError, 'Unable to upload photo'));
        } finally {
            setUploading(false);
            if (fileInputRef.current) fileInputRef.current.value = '';
        }
    };

    const saveProfile = async (): Promise<void> => {
        clearNotice();
        setSaving(true);
        const submittedEmail = email.trim();
        const currentTarget = user?.pending_email ?? user?.email ?? '';
        const wasVerified = Boolean(user?.email_verified_at && user?.email);
        try {
            const payload: { username: string; avatar: string; current_password?: string; email?: string } = {
                username,
                avatar,
            };
            if (user?.password_login_enabled) payload.current_password = profilePassword;
            if (submittedEmail) payload.email = submittedEmail;
            await api.patch('/auth/profile', payload);
            setProfilePassword('');
            await refresh();
            // A changed address becomes a pending claim, not a replacement
            // verified email: the verified recovery address stays active until
            // the new claim is promoted by a successful verification.
            if (submittedEmail && submittedEmail !== currentTarget) {
                try {
                    await api.post('/auth/verify/request');
                    setMessage(
                        wasVerified
                            ? `Verification sent to ${submittedEmail}. Your verified email (${user?.email ?? ''}) stays active until the new address is verified.`
                            : `Verification sent to ${submittedEmail}. Your recovery email activates once verified.`,
                    );
                } catch {
                    setMessage(
                        'Profile updated. The verification email could not be sent — use “Resend verification email” below.',
                    );
                }
            } else {
                setMessage('Profile updated.');
            }
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

    const linkSocialLogin = async (): Promise<void> => {
        clearNotice();
        setLinking(true);
        try {
            await api.post('/auth/oidc/link');
            sessionStorage.setItem('geoguessme_oidc_return_to', '/settings');
            window.location.assign('/oauth2/start?rd=%2Fauth%2Foidc%2Fcallback');
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to start Keycloak login setup'));
            setLinking(false);
        }
    };

    const removeAccount = async (): Promise<void> => {
        if (!window.confirm('Delete your account and gameplay data?')) return;
        clearNotice();
        try {
            const data = user?.password_login_enabled ? { password } : { confirmation: password };
            await api.delete('/auth/account', { data });
            if (typeof fetch === 'function') {
                await fetch('/oauth2/sign_out', { credentials: 'include', redirect: 'manual' }).catch(() => undefined);
            }
            await refresh();
            navigate('/', { replace: true });
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
                        <p className="account-eyebrow">Your GeoGuessMe account</p>
                        <h1 className="auth-title">Settings</h1>
                        <p className="account-intro">Manage your profile, security, and account.</p>
                    </div>
                </div>

                {migrationRequired && (
                    <div className="account-section account-migration-required" role="status">
                        <div className="account-section-heading">
                            <h2>Finish account migration</h2>
                            <p>
                                This legacy account is read-only. Connect Keycloak to the same player ID to restore
                                normal access without moving or recreating your groups, scores, or history.
                            </p>
                        </div>
                        <button className="btn btn-primary" disabled={linking} onClick={() => void linkSocialLogin()}>
                            {linking ? 'Opening secure login…' : 'Connect a Keycloak login'}
                        </button>
                    </div>
                )}

                <div className="account-section" hidden={migrationRequired}>
                    <div className="account-section-heading">
                        <h2>Profile</h2>
                        <p>How friends see you in groups and results.</p>
                    </div>
                    <div className="avatar-preview">
                        <Avatar
                            key={avatarVersion}
                            userID={user?.id ?? ''}
                            avatar={avatar}
                            username={user?.username}
                            className="avatar-preview-img"
                        />
                        <label className={`btn btn-secondary avatar-upload${uploading ? ' is-loading' : ''}`}>
                            {uploading ? 'Uploading…' : 'Upload a photo'}
                            <input
                                ref={fileInputRef}
                                type="file"
                                accept="image/*"
                                hidden
                                onChange={(event) => void uploadPhoto(event)}
                                disabled={uploading}
                            />
                        </label>
                    </div>
                    <p className="account-help">
                        Camera photos are accepted and resized automatically, just like group photos.
                    </p>
                    {avatarError && (
                        <p className="avatar-upload-error" role="alert" aria-live="assertive">
                            {avatarError}
                        </p>
                    )}
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
                    <label htmlFor="settings-email">Recovery email</label>
                    <input
                        id="settings-email"
                        type="email"
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                    />
                    <p className="account-help">
                        Email is a recovery/contact channel, not an identity. A new address is verified before it
                        becomes active.
                    </p>
                    {user?.password_login_enabled && (
                        <>
                            <label htmlFor="profile-current-password">Current password to save profile changes</label>
                            <input
                                id="profile-current-password"
                                type="password"
                                autoComplete="current-password"
                                value={profilePassword}
                                onChange={(event) => setProfilePassword(event.target.value)}
                            />
                        </>
                    )}
                    <button className="btn btn-primary" disabled={saving} onClick={() => void saveProfile()}>
                        Save profile
                    </button>
                </div>

                <div className="account-section" hidden={migrationRequired}>
                    <div className="account-section-heading">
                        <h2>Sign-in methods</h2>
                        <p>
                            Email, Google, Apple, and GitHub sign-in are managed through Keycloak while GeoGuessMe keeps
                            the same player ID and game history.
                        </p>
                    </div>
                    {user?.oidc_linked ? (
                        <>
                            <p className="account-identity-status" role="status">
                                Keycloak login is connected.
                            </p>
                            <p className="account-help">
                                Two-factor authentication, recovery codes, and passkeys are optional. You can add or
                                remove them in your Keycloak account.
                            </p>
                            {keycloakAccountURL && (
                                <a
                                    className="btn btn-secondary"
                                    href={keycloakAccountURL}
                                    target="_blank"
                                    rel="noreferrer"
                                >
                                    Manage 2FA and passkeys
                                </a>
                            )}
                        </>
                    ) : (
                        <button className="btn btn-secondary" disabled={linking} onClick={() => void linkSocialLogin()}>
                            {linking ? 'Opening secure login…' : 'Connect a Keycloak login'}
                        </button>
                    )}
                    {!user?.oidc_linked && user?.password_login_enabled ? (
                        <>
                            <label htmlFor="new-password">New password</label>
                            <input
                                id="new-password"
                                type="password"
                                autoComplete="new-password"
                                value={newPassword}
                                onChange={(event) => setNewPassword(event.target.value)}
                            />
                            <p className="account-help">
                                Use at least 8 characters with uppercase, lowercase, and a number.
                            </p>
                            <button
                                className="btn btn-secondary"
                                disabled={saving}
                                onClick={() => void changePassword()}
                            >
                                Change password
                            </button>
                        </>
                    ) : (
                        <p className="account-help">
                            Normal sign-in goes through Keycloak; legacy password login is disabled.
                        </p>
                    )}
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
                <div className="account-verification" hidden={migrationRequired}>
                    <div>
                        {user?.email_verified_at && user?.email ? (
                            <>
                                <strong>Verified recovery email</strong>
                                <span>{user.email} is confirmed and stays active until a replacement is verified.</span>
                            </>
                        ) : (
                            <strong>No verified email</strong>
                        )}
                        {user?.pending_email ? (
                            <>
                                <strong className="verification-pending-title">Pending verification</strong>
                                <span>Verification was requested for {user.pending_email}.</span>
                            </>
                        ) : (
                            !user?.email_verified_at && (
                                <span>
                                    Email is a recovery/contact channel, not an identity. Add an email to enable account
                                    recovery.
                                </span>
                            )
                        )}
                    </div>
                    {user?.pending_email && (
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
                    <label htmlFor="delete-password">
                        {user?.password_login_enabled
                            ? 'Confirm password to delete account'
                            : `Type ${user?.username ?? 'your username'} to delete account`}
                    </label>
                    <input
                        id="delete-password"
                        type={user?.password_login_enabled ? 'password' : 'text'}
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
