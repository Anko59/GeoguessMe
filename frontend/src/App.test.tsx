import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
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
            setAccessToken: vi.fn(),
        },
    };
});

vi.mock('./api', () => mockModule);

import App from './App';
import Home from './pages/home/Home';

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
});

describe('App shell — public routes', () => {
    it('renders the home page at /', () => {
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByText('geoguess.me')).toBeInTheDocument();
    });

    it('renders the login page at /login', () => {
        routeRef.current = '/login';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByPlaceholderText('Username')).toBeInTheDocument();
    });

    it('renders the signup page at /signup', () => {
        routeRef.current = '/signup';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByPlaceholderText('Email')).toBeInTheDocument();
        expect(screen.getByText('Join the Fun!')).toBeInTheDocument();
    });

    it('renders the forgot-password page at /forgot-password', () => {
        routeRef.current = '/forgot-password';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByLabelText('Email')).toBeInTheDocument();
        expect(screen.getByText('Send reset link')).toBeInTheDocument();
    });

    it('renders the reset-password page at /reset-password', () => {
        routeRef.current = '/reset-password';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByLabelText('New password')).toBeInTheDocument();
        expect(screen.getByText('Reset password')).toBeInTheDocument();
    });

    it('renders the verify-email page at /verify-email', () => {
        routeRef.current = '/verify-email';
        window.history.pushState({}, '', routeRef.current);
        render(<App />);
        expect(screen.getByText('Verification token is missing.')).toBeInTheDocument();
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
