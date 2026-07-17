import { useState } from 'react';
import { Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../api';
import './Auth.css';

export default function ForgotPassword() {
    const [email, setEmail] = useState('');
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const submit = async (event: React.FormEvent): Promise<void> => {
        event.preventDefault(); setError('');
        try { const response = await api.post<{ message: string }>('/auth/password/forgot', { email }); setMessage(response.data.message); }
        catch (requestError: unknown) { setError(getAPIErrorMessage(requestError, 'Unable to request a reset link')); }
    };
    return <div className="auth-container"><div className="auth-card fade-in"><h2 className="auth-title gradient-text">Reset password</h2><p className="auth-subtitle">Enter your email and we’ll send a link if an account matches.</p><form onSubmit={submit} className="auth-form"><label htmlFor="forgot-email">Email</label><input id="forgot-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} required autoComplete="email" />{message && <div className="auth-success" role="status">{message}</div>}{error && <div className="auth-error" role="alert">{error}</div>}<button className="btn btn-primary" type="submit">Send reset link</button></form><p className="auth-footer"><Link to="/login" className="auth-link">Back to login</Link></p></div></div>;
}
