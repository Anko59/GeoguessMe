import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import App from './App';
import Home from './pages/home/Home';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
}));

vi.mock('./api', () => ({
    default: { post: mocks.post, get: vi.fn(), delete: vi.fn() },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
    getAccessToken: () => null,
    setAccessToken: vi.fn(),
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.post.mockReset();
    // AuthProvider calls /auth/refresh on mount; stub it as expired so it
    // finishes without creating a real session.
    mocks.post.mockRejectedValue(new Error('no session'));
});

describe('Home Page', () => {
    it('renders the home page with correct text', () => {
        render(
            <BrowserRouter>
                <Home />
            </BrowserRouter>,
        );

        expect(screen.getByText('geoguess.me')).toBeInTheDocument();
        expect(screen.getByText('Where Snapchat Meets Geoguessr')).toBeInTheDocument();
        expect(screen.getByText('Already Playing? Login')).toBeInTheDocument();
        expect(screen.getByText("Get Started - It's Free!")).toBeInTheDocument();
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
