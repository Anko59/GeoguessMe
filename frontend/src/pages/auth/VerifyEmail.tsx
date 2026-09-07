import { useEffect, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import './Auth.css';

export default function VerifyEmail() {
    const [params] = useSearchParams();
    const token = params.get('token');
    const recovery = params.get('next') === 'password-reset';
    const [verifiedToken, setVerifiedToken] = useState<string | null>(null);
    const [failure, setFailure] = useState<{ token: string; message: string } | null>(null);
    const verified = verifiedToken === token;
    const message = !token
        ? 'Verification token is missing.'
        : verified
          ? 'Email verified.'
          : failure?.token === token
            ? failure.message
            : 'Verifying…';
    useEffect(() => {
        if (!token) return;
        let active = true;
        void api
            .post('/auth/verify', { token })
            .then(() => {
                if (!active) return;
                setVerifiedToken(token);
            })
            .catch((error: unknown) => {
                if (active)
                    setFailure({
                        token,
                        message: getAPIErrorMessage(error, 'Verification link is invalid or expired.'),
                    });
            });
        return () => {
            active = false;
        };
    }, [token]);
    return (
        <div className="auth-container">
            <div className="auth-card fade-in">
                <h2 className="auth-title gradient-text">Email verification</h2>
                <p role="status">{message}</p>
                <p className="auth-footer">
                    {verified && recovery ? (
                        <Link to="/forgot-password" className="auth-link">
                            Request a password reset
                        </Link>
                    ) : (
                        <Link to="/groups" className="auth-link">
                            Continue to GeoGuessMe
                        </Link>
                    )}
                </p>
            </div>
        </div>
    );
}
