import { useCallback, useEffect, useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { exchangeOIDCSession, getAPIErrorCode, getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { AuthResponse } from '../../types';
import './Auth.css';

export default function OIDCCallback() {
    const [error, setError] = useState('');
    const [usernameRequired, setUsernameRequired] = useState(false);
    const [username, setUsername] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const navigate = useNavigate();
    const { login } = useAuth();
    const finishSignIn = useCallback(
        (response: AuthResponse) => {
            login(response);
            const stored = sessionStorage.getItem('geoguessme_oidc_return_to');
            sessionStorage.removeItem('geoguessme_oidc_return_to');
            navigate(stored === '/group/join' || stored === '/settings' ? stored : '/groups', { replace: true });
        },
        [login, navigate],
    );

    useEffect(() => {
        let active = true;
        void exchangeOIDCSession()
            .then((response) => active && finishSignIn(response))
            .catch((requestError: unknown) => {
                if (!active) return;
                if (getAPIErrorCode(requestError) === 'username_required') {
                    setUsernameRequired(true);
                    return;
                }
                setError(getAPIErrorMessage(requestError, 'Secure sign-in failed'));
            });
        return () => {
            active = false;
        };
    }, [finishSignIn]);

    const chooseUsername = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
        event.preventDefault();
        setSubmitting(true);
        setError('');
        try {
            finishSignIn(await exchangeOIDCSession(username.trim()));
        } catch (requestError: unknown) {
            setError(getAPIErrorMessage(requestError, 'Unable to create your player profile'));
        } finally {
            setSubmitting(false);
        }
    };

    return (
        <main className="auth-container">
            <section className="auth-card fade-in" aria-live="polite">
                <img src="/logo.png" alt="GeoGuessMe" className="auth-logo" />
                <h1 className="auth-title gradient-text">
                    {usernameRequired ? 'Choose your username' : 'Finishing sign in…'}
                </h1>
                {usernameRequired ? (
                    <form className="auth-form" onSubmit={(event) => void chooseUsername(event)}>
                        <p className="auth-subtitle">
                            Your identity is verified. Pick the name your friends will see in GeoGuessMe.
                        </p>
                        <label htmlFor="oidc-username">Username</label>
                        <input
                            id="oidc-username"
                            name="username"
                            autoComplete="nickname"
                            autoFocus
                            required
                            minLength={3}
                            maxLength={30}
                            pattern="[A-Za-z0-9_-]+"
                            value={username}
                            onChange={(event) => setUsername(event.target.value)}
                        />
                        <p className="auth-hint">3–30 characters: letters, numbers, underscores, or hyphens.</p>
                        {error && (
                            <p className="auth-error" role="alert">
                                {error}
                            </p>
                        )}
                        <button className="btn btn-primary" type="submit" disabled={submitting}>
                            {submitting ? 'Creating profile…' : 'Start playing'}
                        </button>
                    </form>
                ) : !error ? (
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
