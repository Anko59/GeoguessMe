import { useState } from 'react';
import { useNavigate, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { AuthResponse } from '../../types';
import './Auth.css';

export default function Login() {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const navigate = useNavigate();
    const location = useLocation();
    const { login } = useAuth();
    const returnTo =
        typeof location.state?.from === 'string' && /^\/group\/join(?:\?code=|$)/.test(location.state.from)
            ? location.state.from
            : '/groups';

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
                <p className="auth-subtitle">Login to continue guessing</p>
                <form onSubmit={handleSubmit} className="auth-form">
                    <label htmlFor="login-username">Username</label>
                    <input
                        id="login-username"
                        type="text"
                        placeholder="Username"
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
                        Forgot your password?
                    </Link>
                </p>
                <p className="auth-footer">
                    Don't have an account?{' '}
                    <Link to="/signup" state={{ from: returnTo }} className="auth-link">
                        Sign up
                    </Link>
                </p>
            </div>
        </div>
    );
}
