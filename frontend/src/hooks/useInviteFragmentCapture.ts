import { useLayoutEffect } from 'react';
import { useLocation } from 'react-router-dom';

/** SessionStorage key holding a pending group-invite bearer token. */
export const PENDING_INVITE_TOKEN_KEY = 'pending_invite_token';

const INVITE_FRAGMENT_RE = /^#invite=([^&]+)/;

/**
 * Captures an invite token carried in the URL fragment (`#invite=TOKEN`) into
 * sessionStorage and strips the fragment from the address bar with
 * `history.replaceState`. It runs on every location change, before paint, so
 * the token survives the login/signup redirect even though GroupJoin sits
 * behind ProtectedRoute. The token therefore never travels in a query string,
 * router path param, or server log; GroupJoin consumes it from sessionStorage
 * and submits it only in an HTTPS request body.
 */
export function useInviteFragmentCapture(): void {
    const location = useLocation();
    useLayoutEffect(() => {
        if (typeof window === 'undefined' || typeof window.sessionStorage === 'undefined') return;
        const hash = window.location.hash;
        if (!hash) return;
        const match = INVITE_FRAGMENT_RE.exec(hash);
        if (!match) return;
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, match[1]);
        // Strip only the fragment; the token must never remain in the URL.
        window.history.replaceState(null, '', window.location.pathname + window.location.search);
    }, [location]);
}
