import { useLayoutEffect } from 'react';
import { useLocation } from 'react-router-dom';

/** SessionStorage key holding a pending group-invite bearer token. */
export const PENDING_INVITE_TOKEN_KEY = 'pending_invite_token';

// A 32-byte RawURL base64 token has 42 arbitrary characters followed by a
// character whose two padding bits are zero (an alphabet index divisible by 4).
const INVITE_TOKEN_PATTERN = '[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]';
const INVITE_TOKEN_RE = new RegExp(`^${INVITE_TOKEN_PATTERN}$`);
const INVITE_FRAGMENT_RE = new RegExp(`^#invite=(${INVITE_TOKEN_PATTERN})$`);

/** Returns a canonical token from the current fragment without mutating it. */
export function readInviteFragmentToken(): string | null {
    if (typeof window === 'undefined') return null;
    return INVITE_FRAGMENT_RE.exec(window.location.hash)?.[1] ?? null;
}

/** Reports whether the current URL carries any invite fragment. */
export function hasInviteFragment(): boolean {
    return typeof window !== 'undefined' && window.location.hash.startsWith('#invite=');
}

/** Reads a canonical pending token without letting blocked storage break UI. */
export function readPendingInviteToken(): string | null {
    try {
        const token = window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY);
        return token && INVITE_TOKEN_RE.test(token) ? token : null;
    } catch {
        return null;
    }
}

/** Clears a pending token when storage is available. */
export function clearPendingInviteToken(): void {
    try {
        window.sessionStorage.removeItem(PENDING_INVITE_TOKEN_KEY);
    } catch {
        // Storage can be disabled by browser privacy settings.
    }
}

function storePendingInviteToken(token: string): void {
    try {
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, token);
    } catch {
        // The fragment is still stripped below so bearer data never lingers.
    }
}

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
        if (typeof window === 'undefined') return;
        const hash = window.location.hash;
        if (!hash.startsWith('#invite=')) return;
        const token = readInviteFragmentToken();
        if (token) storePendingInviteToken(token);
        // Strip only the fragment; the token must never remain in the URL.
        window.history.replaceState(window.history.state, '', window.location.pathname + window.location.search);
    }, [location]);
}
