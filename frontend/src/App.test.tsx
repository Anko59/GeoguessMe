import { act, fireEvent, render, screen } from '@testing-library/react';
import { BrowserRouter, MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { AuthResponse } from './types';

const { routeRef, apiMocks, mockModule } = vi.hoisted(() => {
    const routeRef = { current: '/' };
    const apiMocks = { get: vi.fn(), post: vi.fn(), delete: vi.fn() };
    return {
        routeRef,
        apiMocks,
        mockModule: {
            default: { get: apiMocks.get, post: apiMocks.post, delete: apiMocks.delete },
            getAPIErrorMessage: (error: unknown, fallback: string) =>
                error instanceof Error ? error.message : fallback,
            getAccessToken: () => null,
            refreshAuthSession: () =>
                apiMocks
                    .post('/auth/refresh')
                    .then((response: { data?: AuthResponse } | undefined) => response?.data ?? null)
                    .catch(() => null),
            exchangeOIDCSession: vi.fn(),
            setAccessToken: vi.fn(),
        },
    };
});

vi.mock('./api', () => mockModule);

import App from './App';
import Home from './pages/home/Home';
import { AuthContext } from './context/AuthContext';

const authResponse: AuthResponse = {
    access_token: 'access-token',
    expires_in: 900,
    user: {
        id: 'u1',
        username: 'alice',
        email: 'alice@example.test',
        avatar: 'avatar.png',
        password_login_enabled: true,
        oidc_linked: false,
        migration_required: false,
    },
};

beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    sessionStorage.clear();
    routeRef.current = '/';
    window.history.pushState({}, '', '/');
    apiMocks.get.mockReset();
    apiMocks.post.mockReset();
    apiMocks.delete.mockReset();
    // By default, fail auth refresh so the shell is in an unauthenticated state.
    apiMocks.post.mockRejectedValue(new Error('no session'));
    // Public route tests exercise the intentionally supported OIDC-off mode.
    apiMocks.get.mockResolvedValue({ data: { enabled: false, login_path: '/oauth2/start', social_providers: [] } });
});

describe('Home Page', () => {
    it('renders the home page with correct text', () => {
        render(
            <BrowserRouter>
                <Home />
            </BrowserRouter>,
        );
        expect(screen.getByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
    });

    it('redirects authenticated visitors to groups', () => {
        render(
            <AuthContext.Provider
                value={{
                    user: authResponse.user,
                    loading: false,
                    isAuthenticated: true,
                    login: vi.fn(),
                    logout: vi.fn(async () => undefined),
                    refresh: vi.fn(async () => true),
                }}
            >
                <MemoryRouter initialEntries={['/']}>
                    <Home />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(screen.queryByRole('heading', { name: /guess the place/i })).not.toBeInTheDocument();
    });

    it('keeps the landing page visible during logout navigation', () => {
        render(
            <AuthContext.Provider
                value={{
                    user: authResponse.user,
                    loading: false,
                    isAuthenticated: true,
                    login: vi.fn(),
                    logout: vi.fn(async () => undefined),
                    refresh: vi.fn(async () => true),
                }}
            >
                <MemoryRouter initialEntries={[{ pathname: '/', state: { loggingOut: true } }]}>
                    <Home />
                </MemoryRouter>
            </AuthContext.Provider>,
        );
        expect(screen.getByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
    });
});

describe('App shell — public routes', () => {
    it('renders the home page at /', async () => {
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
    });

    it('renders the login page at /login', async () => {
        routeRef.current = '/login';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('renders the signup page at /signup', async () => {
        routeRef.current = '/signup';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByPlaceholderText('Email — verify to enable account recovery')).toBeInTheDocument();
        expect(await screen.findByText('Join the Fun!')).toBeInTheDocument();
    });

    it('renders legacy credentials only at /migrate-account', async () => {
        routeRef.current = '/migrate-account';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByRole('heading', { name: 'Migrate your account' })).toBeInTheDocument();
        expect(screen.getByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('renders the forgot-password page at /forgot-password', async () => {
        routeRef.current = '/forgot-password';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByLabelText('Email')).toBeInTheDocument();
        expect(await screen.findByText('Send reset or verification link')).toBeInTheDocument();
    });

    it('renders the reset-password page at /reset-password', async () => {
        routeRef.current = '/reset-password';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByLabelText('New password')).toBeInTheDocument();
        expect(await screen.findByText('Reset password')).toBeInTheDocument();
    });

    it('renders the verify-email page at /verify-email', async () => {
        routeRef.current = '/verify-email';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByText('Verification token is missing.')).toBeInTheDocument();
    });
});

describe('App shell — protected routes redirect when unauthenticated', () => {
    it('redirects /groups to /login', async () => {
        routeRef.current = '/groups';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        // AuthProvider refresh rejects → ProtectedRoute redirects to /login
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('redirects /group/join to /login', async () => {
        routeRef.current = '/group/join';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('redirects /group/create to /login', async () => {
        routeRef.current = '/group/create';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('redirects /group/:id to /login', async () => {
        routeRef.current = '/group/some-id';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });

    it('redirects /settings to /login', async () => {
        routeRef.current = '/settings';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username or email')).toBeInTheDocument();
    });
});

describe('App shell — protected routes with authentication', () => {
    beforeEach(() => {
        // Succeed the auth refresh so the user is authenticated.
        apiMocks.post.mockReset();
        apiMocks.post.mockResolvedValue({ data: authResponse });
        apiMocks.get.mockReset();
        apiMocks.get.mockResolvedValue({ data: [] });
    });

    it('renders groups list at /groups', async () => {
        routeRef.current = '/groups';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        // Home redirects authenticated users to /groups, GroupsList fetches groups
        expect(await screen.findByText("You haven't joined any groups yet")).toBeInTheDocument();
    });

    it('renders group join/create page at /group/join', async () => {
        routeRef.current = '/group/join';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        // No pending invite token: the join flow shows the missing-invite state.
        expect(await screen.findByText('No invite link found')).toBeInTheDocument();
    });

    it('captures an invite fragment to sessionStorage and strips it from the URL', async () => {
        apiMocks.post.mockReset();
        apiMocks.get.mockReset();
        apiMocks.post.mockImplementation((url: string) => {
            if (url === '/auth/refresh') return Promise.resolve({ data: authResponse });
            if (url === '/group/invites/preview') {
                return Promise.resolve({ data: { group_name: 'Friends', member_count: 3 } });
            }
            return Promise.reject(new Error('unexpected POST ' + url));
        });
        apiMocks.get.mockResolvedValue({ data: [] });
        const inviteToken = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA';
        routeRef.current = `/group/join#invite=${inviteToken}`;
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        // GroupJoin previews the invite token it read from sessionStorage.
        expect(await screen.findByText('Join Friends?')).toBeInTheDocument();
        expect(sessionStorage.getItem('pending_invite_token')).toBe(inviteToken);
        // The fragment is stripped from the address bar; the token stays in sessionStorage.
        expect(window.location.hash).toBe('');
    });

    it('renders settings at /settings', async () => {
        routeRef.current = '/settings';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument();
    });

    it('renders the settings route when reached through authenticated navigation', async () => {
        routeRef.current = '/groups';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByRole('heading', { name: 'My Groups' })).toBeInTheDocument();

        fireEvent.click(screen.getByRole('link', { name: 'Settings' }));

        expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument();
        expect(window.location.pathname).toBe('/settings');
    });

    it('renders the public landing page immediately after logout', async () => {
        routeRef.current = '/settings';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByRole('heading', { name: 'Settings' })).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'Logout' }));

        expect(await screen.findByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
        expect(screen.queryByRole('heading', { name: 'Settings' })).not.toBeInTheDocument();
        expect(window.location.pathname).toBe('/');
    });
});

describe('App startup', () => {
    it('mounts the full App with Router and AuthProvider', async () => {
        render(<App />);
        // AuthProvider fires a session-restore POST on mount; the mock
        // rejects it so the Home route renders as an unauthenticated visitor.
        expect(await screen.findByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
    });

    it('shows the home page logo and welcome assets', async () => {
        render(<App />);
        expect(await screen.findByRole('heading', { name: /geoguess\.me.*guess the place/i })).toBeInTheDocument();
        expect(document.querySelector('.welcome-asset-img')).toHaveAttribute('alt', '');
        expect(screen.getByRole('heading', { name: 'Snap' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'Guess' })).toBeInTheDocument();
        expect(screen.getByRole('heading', { name: 'Climb' })).toBeInTheDocument();
    });

    it('provides signup and login navigation links', async () => {
        render(<App />);
        await screen.findByRole('heading', { name: /geoguess\.me.*guess the place/i });
        expect(screen.getByText("Get Started - It's Free!").closest('a')).toHaveAttribute('href', '/signup');
        expect(screen.getByText('Already Playing? Login').closest('a')).toHaveAttribute('href', '/login');
    });
});
