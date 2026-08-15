import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { exchangeOIDCSession, getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import './Auth.css';

export default function OIDCCallback() {
    const [error, setError] = useState('');
    const navigate = useNavigate();
    const { login } = useAuth();

    useEffect(() => {
        let active = true;
        void exchangeOIDCSession()
            .then((response) => {
                if (!active) return;
                login(response);
                const stored = sessionStorage.getItem('geoguessme_oidc_return_to');
                sessionStorage.removeItem('geoguessme_oidc_return_to');
                navigate(stored === '/group/join' || stored === '/settings' ? stored : '/groups', { replace: true });
            })
            .catch((requestError: unknown) => {
                if (active) setError(getAPIErrorMessage(requestError, 'Social login failed'));
            });
        return () => {
            active = false;
        };
    }, [login, navigate]);

    return (
        <main className="auth-container">
            <section className="auth-card fade-in" aria-live="polite">
                <img src="/logo.png" alt="GeoGuessMe" className="auth-logo" />
                <h1 className="auth-title gradient-text">Finishing sign in…</h1>
                {!error ? (
                    <p className="auth-subtitle">Securely connecting your GeoGuessMe account.</p>
                ) : (
                    <>
                        <p className="auth-error" role="alert">
                            {error}
                        </p>
                        <p className="auth-subtitle">
                            If you already play with a username, use the one-time migration sign-in and connect Keycloak
                            from Settings. Your groups and scores will stay on that account.
                        </p>
                        <Link className="btn btn-primary" to="/migrate-account">
                            Migrate existing account
                        </Link>
                    </>
                )}
            </section>
        </main>
    );
}
