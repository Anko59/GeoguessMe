import { useEffect, useState } from 'react';
import { useNavigate, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { AuthResponse, OIDCConfig } from '../../types';
import './Auth.css';

export default function Signup() {
    const [username, setUsername] = useState('');
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [oidcConfig, setOIDCConfig] = useState<OIDCConfig | null>(null);
    const navigate = useNavigate();
    const location = useLocation();
    const { login } = useAuth();
    const rawFrom = typeof location.state?.from === 'string' ? location.state.from : '';
    // Strip any query string or fragment: the invite token never travels in a
    // URL query. GroupJoin reads it from sessionStorage after signup, so only a
    // bare /group/join is a valid post-auth target.
    const fromPath = rawFrom.split('?')[0].split('#')[0];
    const returnTo = fromPath === '/group/join' ? '/group/join' : '/groups';

    useEffect(() => {
        let active = true;
        void api
            .get<OIDCConfig>('/auth/oidc/config')
            .then((response) => {
                if (active) setOIDCConfig(response.data);
            })
            .catch(() => undefined);
        return () => {
            active = false;
        };
    }, []);

    const rememberSocialReturn = (): void => {
        sessionStorage.setItem('geoguessme_oidc_return_to', returnTo);
    };

    const handleSubmit = async (event: React.FormEvent): Promise<void> => {
        event.preventDefault();
        setError('');
        setSubmitting(true);
        const payload: { username: string; password: string; email?: string } = { username, password };
        if (email.trim()) payload.email = email.trim();
        try {
            const response = await api.post<AuthResponse>('/auth/signup', payload);
            login(response.data);
            navigate(returnTo, { replace: true });
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Signup failed'));
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="auth-container">
            <div className="auth-card fade-in">
                <img src="/logo.png" alt="GeoGuessMe" className="auth-logo" />
                <h2 className="auth-title gradient-text">Join the Fun!</h2>
                <p className="auth-subtitle">Create your secure account to start</p>
                {oidcConfig?.enabled ? (
                    <>
                        <a
                            className="btn btn-social"
                            href={`${oidcConfig.login_path}?rd=${encodeURIComponent('/auth/oidc/callback')}`}
                            onClick={rememberSocialReturn}
                        >
                            Sign up with Google, Apple, or GitHub
                        </a>
                        <p className="auth-provider-note">
                            Keycloak creates your GeoGuessMe account after your provider verifies you. Two-factor
                            authentication and passkeys stay optional.
                        </p>
                    </>
                ) : oidcConfig ? (
                    <form onSubmit={handleSubmit} className="auth-form">
                        <label htmlFor="signup-username">Username</label>
                        <input
                            id="signup-username"
                            type="text"
                            placeholder="Username"
                            value={username}
                            onChange={(event) => setUsername(event.target.value)}
                            required
                            autoComplete="username"
                        />
                        <label htmlFor="signup-email">Recovery email (optional)</label>
                        <input
                            id="signup-email"
                            type="email"
                            placeholder="Email — verify to enable account recovery"
                            value={email}
                            onChange={(event) => setEmail(event.target.value)}
                            autoComplete="email"
                        />
                        <p className="auth-hint">
                            Email is a recovery/contact channel, not an identity — an optional address you can verify
                            later.
                        </p>
                        <label htmlFor="signup-password">Password</label>
                        <input
                            id="signup-password"
                            type="password"
                            placeholder="Password"
                            value={password}
                            onChange={(event) => setPassword(event.target.value)}
                            required
                            autoComplete="new-password"
                        />
                        <p className="auth-hint">Use at least 8 characters with uppercase, lowercase, and a number.</p>
                        {error && (
                            <div className="auth-error" role="alert">
                                {error}
                            </div>
                        )}
                        <button type="submit" className="btn btn-primary" disabled={submitting}>
                            {submitting ? 'Creating account…' : 'Sign Up'}
                        </button>
                    </form>
                ) : (
                    <p className="auth-provider-note" role="status">
                        Loading secure signup…
                    </p>
                )}
                <p className="auth-footer">
                    Already have an account?{' '}
                    <Link to="/login" state={{ from: returnTo }} className="auth-link">
                        Login
                    </Link>
                </p>
            </div>
        </div>
    );
}
