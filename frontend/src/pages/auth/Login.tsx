import { useEffect, useState } from 'react';
import { useNavigate, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { AuthResponse, OIDCConfig } from '../../types';
import OIDCOptions from './OIDCOptions';
import './Auth.css';

export default function Login({ existingAccountMode = false }: { existingAccountMode?: boolean }) {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [oidcConfig, setOIDCConfig] = useState<OIDCConfig | null>(null);
    const navigate = useNavigate();
    const location = useLocation();
    const { login } = useAuth();
    const rawFrom = typeof location.state?.from === 'string' ? location.state.from : '';
    // Strip any query string or fragment: the invite token never travels in a
    // URL query. GroupJoin reads it from sessionStorage after login, so only a
    // bare /group/join is a valid post-auth target.
    const fromPath = rawFrom.split('?')[0].split('#')[0];
    const returnTo = existingAccountMode ? '/settings' : fromPath === '/group/join' ? '/group/join' : '/groups';
    const activeOIDCConfig = existingAccountMode
        ? { enabled: false, login_path: '/oauth2/start', social_providers: [] }
        : oidcConfig;

    useEffect(() => {
        if (existingAccountMode) {
            return;
        }
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
    }, [existingAccountMode]);

    const rememberSocialReturn = (): void => {
        sessionStorage.setItem('geoguessme_oidc_return_to', returnTo);
    };

    const handleSubmit = async (event: React.FormEvent): Promise<void> => {
        event.preventDefault();
        setError('');
        setSubmitting(true);
        try {
            const response = await api.post<AuthResponse>('/auth/login', { username, password });
            login(response.data);
            navigate(returnTo, { replace: true });
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Login failed'));
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <div className="auth-container">
            <div className="auth-card fade-in">
                <img src="/logo.png" alt="GeoGuessMe" className="auth-logo" />
                <h2 className="auth-title gradient-text">Welcome Back!</h2>
                <p className="auth-subtitle">
                    {existingAccountMode
                        ? 'Sign in with your existing username or email and password.'
                        : 'Use your existing GeoGuessMe account or continue with Google.'}
                </p>
                {existingAccountMode && (
                    <p className="auth-account-notice" role="note">
                        Your existing account works as usual. Connecting Google in Settings is optional.
                    </p>
                )}
                <form onSubmit={handleSubmit} className="auth-form">
                    <label htmlFor="login-username">Username or email</label>
                    <input
                        id="login-username"
                        type="text"
                        placeholder="Username or email"
                        value={username}
                        onChange={(event) => setUsername(event.target.value)}
                        required
                        autoComplete="username"
                    />
                    <label htmlFor="login-password">Password</label>
                    <input
                        id="login-password"
                        type="password"
                        placeholder="Password"
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        required
                        autoComplete="current-password"
                    />
                    {error && (
                        <div className="auth-error" role="alert">
                            {error}
                        </div>
                    )}
                    <button type="submit" className="btn btn-primary" disabled={submitting}>
                        {submitting ? 'Logging in…' : 'Login'}
                    </button>
                </form>
                <p className="auth-footer">
                    <Link to="/forgot-password" className="auth-link">
                        Forgot your username or password?
                    </Link>
                </p>
                {activeOIDCConfig?.enabled && (
                    <>
                        <div className="auth-divider">or</div>
                        <OIDCOptions
                            loginPath={activeOIDCConfig.login_path}
                            intent="login"
                            onStart={rememberSocialReturn}
                            socialProviders={activeOIDCConfig.social_providers}
                        />
                    </>
                )}
                <p className="auth-footer">
                    {existingAccountMode ? (
                        <Link to="/login" className="auth-link">
                            Back to sign in
                        </Link>
                    ) : (
                        <>
                            Don't have an account?{' '}
                            <Link to="/signup" state={{ from: returnTo }} className="auth-link">
                                Sign up
                            </Link>
                        </>
                    )}
                </p>
            </div>
        </div>
    );
}
