import { render } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { BrowserRouter } from 'react-router-dom';
import {
    clearPendingInviteToken,
    PENDING_INVITE_TOKEN_KEY,
    readPendingInviteToken,
    useInviteFragmentCapture,
} from './useInviteFragmentCapture';

const inviteToken = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';

function CaptureInviteFragment() {
    useInviteFragmentCapture();
    return null;
}

beforeEach(() => {
    window.sessionStorage.clear();
    window.history.replaceState({}, '', '/');
});

afterEach(() => {
    vi.restoreAllMocks();
});

describe('invite fragment capture', () => {
    it('stores a canonical token, strips the fragment, and preserves router state', () => {
        const state = { key: 'router-state' };
        window.history.pushState(state, '', `/group/join#invite=${inviteToken}`);
        render(
            <BrowserRouter>
                <CaptureInviteFragment />
            </BrowserRouter>,
        );

        expect(window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBe(inviteToken);
        expect(window.location.hash).toBe('');
        expect(window.history.state).toMatchObject(state);
    });

    it('strips a malformed invite fragment without replacing a pending valid token', () => {
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, inviteToken);
        window.history.pushState({}, '', '/group/join#invite=malformed');
        render(
            <BrowserRouter>
                <CaptureInviteFragment />
            </BrowserRouter>,
        );

        expect(window.location.hash).toBe('');
        expect(window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBe(inviteToken);
    });

    it('rejects a 43-character token with non-canonical base64 padding bits', () => {
        const nonCanonicalToken = `${inviteToken.slice(0, -1)}B`;
        window.history.pushState({}, '', `/group/join#invite=${nonCanonicalToken}`);
        render(
            <BrowserRouter>
                <CaptureInviteFragment />
            </BrowserRouter>,
        );

        expect(window.location.hash).toBe('');
        expect(window.sessionStorage.getItem(PENDING_INVITE_TOKEN_KEY)).toBeNull();
    });

    it('keeps navigation usable when sessionStorage is blocked', () => {
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new DOMException('blocked');
        });
        window.history.pushState({}, '', `/group/join#invite=${inviteToken}`);

        expect(() =>
            render(
                <BrowserRouter>
                    <CaptureInviteFragment />
                </BrowserRouter>,
            ),
        ).not.toThrow();
        expect(window.location.hash).toBe('');
    });
});

describe('pending invite storage helpers', () => {
    it('rejects malformed stored values and tolerates blocked storage', () => {
        window.sessionStorage.setItem(PENDING_INVITE_TOKEN_KEY, 'malformed');
        expect(readPendingInviteToken()).toBeNull();

        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
            throw new DOMException('blocked');
        });
        expect(readPendingInviteToken()).toBeNull();
        expect(clearPendingInviteToken).not.toThrow();
    });
});
