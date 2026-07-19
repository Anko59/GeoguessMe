import { useState } from 'react';
import { useNavigate, Link, useLocation } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { AuthResponse } from '../../types';
import './Auth.css';

export default function Signup() {
    const [username, setUsername] = useState('');
    const [email, setEmail] = useState('');
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
                <p className="auth-subtitle">Create your account to start</p>
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
                    <label htmlFor="signup-email">Email</label>
                    <input
                        id="signup-email"
                        type="email"
                        placeholder="Email"
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                        autoComplete="email"
                    />
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
