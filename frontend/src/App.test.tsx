import { act, render, screen } from '@testing-library/react';
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
    user: { id: 'u1', username: 'alice', email: 'alice@example.test', avatar: 'avatar.png' },
};

beforeEach(() => {
    vi.clearAllMocks();
    routeRef.current = '/';
    window.history.pushState({}, '', '/');
    apiMocks.get.mockReset();
    apiMocks.post.mockReset();
    apiMocks.delete.mockReset();
    // By default, fail auth refresh so the shell is in an unauthenticated state.
    apiMocks.post.mockRejectedValue(new Error('no session'));
});

describe('Home Page', () => {
    it('renders the home page with correct text', () => {
        render(
            <BrowserRouter>
                <Home />
            </BrowserRouter>,
        );
        expect(screen.getByText('geoguess.me')).toBeInTheDocument();
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
        expect(screen.queryByText('geoguess.me')).not.toBeInTheDocument();
    });
});

describe('App shell — public routes', () => {
    it('renders the home page at /', async () => {
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByText('geoguess.me')).toBeInTheDocument();
    });

    it('renders the login page at /login', async () => {
        routeRef.current = '/login';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('renders the signup page at /signup', async () => {
        routeRef.current = '/signup';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByPlaceholderText('Email')).toBeInTheDocument();
        expect(await screen.findByText('Join the Fun!')).toBeInTheDocument();
    });

    it('renders the forgot-password page at /forgot-password', async () => {
        routeRef.current = '/forgot-password';
        window.history.pushState({}, '', routeRef.current);
        await act(async () => {
            render(<App />);
        });
        expect(await screen.findByLabelText('Email')).toBeInTheDocument();
        expect(await screen.findByText('Send reset link')).toBeInTheDocument();
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
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('redirects /group/join to /login', async () => {
        routeRef.current = '/group/join';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('redirects /group/create to /login', async () => {
        routeRef.current = '/group/create';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('redirects /group/:id to /login', async () => {
        routeRef.current = '/group/some-id';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('redirects /settings to /login', async () => {
        routeRef.current = '/settings';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByPlaceholderText('Username')).toBeInTheDocument();
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
        expect(await screen.findByPlaceholderText('6-character code')).toBeInTheDocument();
    });

    it('renders settings at /settings', async () => {
        routeRef.current = '/settings';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(await screen.findByText('Account settings')).toBeInTheDocument();
    });
});

describe('App startup', () => {
    it('mounts the full App with Router and AuthProvider', async () => {
        render(<App />);
        // AuthProvider fires a session-restore POST on mount; the mock
        // rejects it so the Home route renders as an unauthenticated visitor.
        expect(await screen.findByText('geoguess.me')).toBeInTheDocument();
    });

    it('shows the home page logo and welcome assets', async () => {
        render(<App />);
        expect(await screen.findByAltText('Welcome Banner')).toBeInTheDocument();
        expect(screen.getByAltText('Welcome')).toBeInTheDocument();
        expect(screen.getByText('Share Photos')).toBeInTheDocument();
        expect(screen.getByText('Guess Locations')).toBeInTheDocument();
        expect(screen.getByText('Compete')).toBeInTheDocument();
    });

    it('provides signup and login navigation links', async () => {
        render(<App />);
        await screen.findByText('geoguess.me');
        expect(screen.getByText("Get Started - It's Free!").closest('a')).toHaveAttribute('href', '/signup');
        expect(screen.getByText('Already Playing? Login').closest('a')).toHaveAttribute('href', '/login');
    });
});
