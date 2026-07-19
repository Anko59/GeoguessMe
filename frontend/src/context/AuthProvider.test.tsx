import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuth } from './AuthContext';
import AuthProvider from './AuthProvider';
import type { AuthResponse } from '../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    delete: vi.fn(),
    setAccessToken: vi.fn(),
    refreshAuthSession: vi.fn(),
    token: null as string | null,
}));

vi.mock('../api', () => ({
    default: { get: mocks.get, post: mocks.post, delete: mocks.delete },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
    getAccessToken: () => mocks.token,
    setAccessToken: (token: string | null) => {
        mocks.token = token;
        mocks.setAccessToken(token);
    },
    refreshAuthSession: () => mocks.refreshAuthSession(),
}));

const authResponse: AuthResponse = {
    access_token: 'access-token',
    expires_in: 900,
    user: {
        id: 'user-1',
        username: 'alice',
        email: 'alice@example.test',
        avatar: 'avatar.png',
        email_verified_at: null,
    },
};

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.delete.mockReset();
    mocks.token = null;
    mocks.refreshAuthSession.mockReset();
});

describe('AuthProvider', () => {
    it('restores, logs in, and logs out through AuthProvider', async () => {
        mocks.refreshAuthSession.mockResolvedValueOnce(authResponse);
        mocks.post.mockResolvedValueOnce({ data: {} });
        function Consumer() {
            const auth = useAuth();
            return (
                <>
                    <output>{auth.loading ? 'loading' : (auth.user?.username ?? 'signed-out')}</output>
                    <button onClick={() => auth.login(authResponse)}>Login</button>
                    <button onClick={() => void auth.logout()}>Logout</button>
                </>
            );
        }
        render(
            <MemoryRouter>
                <AuthProvider>
                    <Consumer />
                </AuthProvider>
            </MemoryRouter>,
        );
        expect(await screen.findByText('alice')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Logout' }));
        await waitFor(() => expect(screen.getByText('signed-out')).toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', { name: 'Login' }));
        expect(screen.getByText('alice')).toBeInTheDocument();
        expect(mocks.post).toHaveBeenCalledWith('/auth/logout');
    });

    it('clears a failed restored session and guards useAuth', async () => {
        mocks.refreshAuthSession.mockResolvedValue(null);
        function Consumer() {
            return <output>{useAuth().isAuthenticated ? 'yes' : 'no'}</output>;
        }
        render(
            <MemoryRouter>
                <AuthProvider>
                    <Consumer />
                </AuthProvider>
            </MemoryRouter>,
        );
        expect(await screen.findByText('no')).toBeInTheDocument();
        expect(() => render(<Consumer />)).toThrow('useAuth must be used inside AuthProvider');
    });
});
