import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import './Auth.css';

export default function VerifyEmail() {
    const [params] = useSearchParams(); const token = params.get('token'); const [message, setMessage] = useState(token ? 'Verifying…' : 'Verification token is missing.');
    useEffect(() => { if (!token) return; void api.post('/auth/verify', { token }).then(() => setMessage('Email verified.')).catch((error: unknown) => setMessage(getAPIErrorMessage(error, 'Verification link is invalid or expired.'))); }, [token]);
    return <div className="auth-container"><div className="auth-card fade-in"><h2 className="auth-title gradient-text">Email verification</h2><p role="status">{message}</p><p className="auth-footer"><Link to="/groups" className="auth-link">Continue to GeoGuessMe</Link></p></div></div>;
}
