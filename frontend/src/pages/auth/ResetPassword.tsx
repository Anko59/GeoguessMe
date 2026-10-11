import { useState } from 'react';
import { Link, useSearchParams, useNavigate } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import './Auth.css';

export default function ResetPassword() {
    const [params] = useSearchParams();
    const navigate = useNavigate();
    const [password, setPassword] = useState('');
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const submit = async (event: React.FormEvent): Promise<void> => {
        event.preventDefault();
        setError('');
        try {
            await api.post('/auth/password/reset', { token: params.get('token') ?? '', password });
            setMessage(
                'Direct-login password reset. Sign in with Username or email / Password. Your GeoGuessMe ID password has not changed.',
            );
            setTimeout(() => navigate('/login'), 1200);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to reset password'));
        }
    };
    return (
        <div className="auth-container">
            <div className="auth-card fade-in">
                <h2 className="auth-title gradient-text">Choose a new direct-login password</h2>
                <p className="auth-subtitle">This will not change your separate GeoGuessMe ID password.</p>
                <form onSubmit={submit} className="auth-form">
                    <label htmlFor="reset-password">New password</label>
                    <input
                        id="reset-password"
                        type="password"
                        value={password}
                        onChange={(event) => setPassword(event.target.value)}
                        required
                        autoComplete="new-password"
                    />
                    {message && (
                        <div className="auth-success" role="status">
                            {message}
                        </div>
                    )}
                    {error && (
                        <div className="auth-error" role="alert">
                            {error}
                        </div>
                    )}
                    <button className="btn btn-primary" type="submit">
                        Reset password
                    </button>
                </form>
                <p className="auth-footer">
                    <Link to="/login" className="auth-link">
                        Back to login
                    </Link>
                </p>
            </div>
        </div>
    );
}
