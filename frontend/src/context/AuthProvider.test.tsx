import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import ProtectedRoute from '../components/navigation/ProtectedRoute';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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
        password_login_enabled: true,
        oidc_linked: false,
        migration_required: false,
    },
};

beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    mocks.get.mockReset();
    mocks.post.mockReset();
    mocks.delete.mockReset();
    mocks.token = null;
    mocks.refreshAuthSession.mockReset();
});

afterEach(() => localStorage.clear());

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

    it('removes an open protected page on another tab logout and ignores an in-flight refresh', async () => {
        localStorage.setItem('geoguessme:pwa-session:v1', JSON.stringify(authResponse.user));
        let finishRefresh!: (response: AuthResponse | null) => void;
        mocks.refreshAuthSession.mockReturnValue(new Promise((resolve) => (finishRefresh = resolve)));
        render(
            <MemoryRouter initialEntries={['/settings']}>
                <AuthProvider>
                    <Routes>
                        <Route
                            path="/settings"
                            element={
                                <ProtectedRoute>
                                    <div>Private settings</div>
                                </ProtectedRoute>
                            }
                        />
                        <Route path="/login" element={<div>Sign in</div>} />
                    </Routes>
                </AuthProvider>
            </MemoryRouter>,
        );
        expect(screen.getByText('Private settings')).toBeInTheDocument();
        await waitFor(() => expect(mocks.refreshAuthSession).toHaveBeenCalledTimes(1));
        window.dispatchEvent(new StorageEvent('storage', { key: 'unrelated', newValue: 'changed' }));
        expect(screen.getByText('Private settings')).toBeInTheDocument();
        act(() => {
            window.dispatchEvent(
                new StorageEvent('storage', {
                    key: 'geoguessme:logout:v1',
                    newValue: 'another-tab-logout',
                }),
            );
        });
        expect(await screen.findByText('Sign in')).toBeInTheDocument();
        expect(screen.queryByText('Private settings')).not.toBeInTheDocument();
        expect(localStorage.getItem('geoguessme:pwa-session:v1')).toBeNull();
        await act(async () => finishRefresh(authResponse));
        expect(screen.getByText('Sign in')).toBeInTheDocument();
        expect(mocks.setAccessToken).not.toHaveBeenCalledWith('access-token');
    });

    it('broadcasts logout to other tabs even without a cached hint', async () => {
        mocks.refreshAuthSession.mockResolvedValueOnce(null);
        mocks.post.mockResolvedValueOnce({ data: {} });
        function Consumer() {
            const { logout } = useAuth();
            return <button onClick={() => void logout()}>Logout</button>;
        }
        render(
            <AuthProvider>
                <Consumer />
            </AuthProvider>,
        );
        await waitFor(() => expect(mocks.refreshAuthSession).toHaveBeenCalledTimes(1));
        fireEvent.click(screen.getByRole('button', { name: 'Logout' }));
        await waitFor(() => expect(localStorage.getItem('geoguessme:logout:v1')).not.toBeNull());
    });

    it('renders a cached session immediately while it refreshes in the background', async () => {
        localStorage.setItem('geoguessme:pwa-session:v1', JSON.stringify(authResponse.user));
        mocks.refreshAuthSession.mockResolvedValue(authResponse);
        function Consumer() {
            const auth = useAuth();
            return <output>{auth.loading ? 'loading' : (auth.user?.username ?? 'signed-out')}</output>;
        }

        render(
            <MemoryRouter>
                <AuthProvider>
                    <Consumer />
                </AuthProvider>
            </MemoryRouter>,
        );

        expect(screen.getByText('alice')).toBeInTheDocument();
        await waitFor(() => expect(mocks.refreshAuthSession).toHaveBeenCalledTimes(1));
    });
});
