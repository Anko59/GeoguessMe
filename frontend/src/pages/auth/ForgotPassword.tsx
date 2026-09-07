import { useState } from 'react';
import { Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import './Auth.css';

export default function ForgotPassword() {
    const [email, setEmail] = useState('');
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const submit = async (event: React.FormEvent): Promise<void> => {
        event.preventDefault();
        setError('');
        setMessage('');
        setSubmitting(true);
        try {
            const response = await api.post<{ message: string }>('/auth/password/forgot', { email });
            setMessage(response.data.message);
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to request a reset link'));
        } finally {
            setSubmitting(false);
        }
    };
    return (
        <div className="auth-container">
            <div className="auth-card fade-in">
                <h2 className="auth-title gradient-text">Reset password</h2>
                <p className="auth-subtitle">
                    Enter the email address from your old GeoGuessMe account. We’ll send a reset link if it is verified,
                    or a verification link first if it is not.
                </p>
                <form onSubmit={submit} className="auth-form">
                    <label htmlFor="forgot-email">Email</label>
                    <input
                        id="forgot-email"
                        type="email"
                        value={email}
                        onChange={(event) => setEmail(event.target.value)}
                        required
                        autoComplete="email"
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
                    <button className="btn btn-primary" type="submit" disabled={submitting}>
                        {submitting ? 'Sending…' : 'Send reset or verification link'}
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
