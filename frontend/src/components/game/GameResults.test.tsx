import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AuthContext } from '../../context/AuthContext';
import Game from './Game';
import type { Message, User } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get, post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

vi.mock('../map/Map', () => ({
    default: ({ onLocationSelect }: { onLocationSelect: (lat: number, long: number) => void }) => (
        <button onClick={() => onLocationSelect(48.8, 2.3)}>Map</button>
    ),
}));

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
    email_verified_at: null,
    password_login_enabled: true,
    oidc_linked: false,
    migration_required: false,
};

const authValue = {
    user,
    loading: false,
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(async () => undefined),
    refresh: vi.fn(async () => false),
};

const message = (overrides: Partial<Message> = {}): Message => ({
    id: 'message-1',
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    avatar: 'avatar.png',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
});

beforeEach(() => {
    vi.clearAllMocks();
    vi.unstubAllGlobals();
    mocks.get.mockReset();
    mocks.post.mockReset();
    Element.prototype.scrollIntoView = vi.fn();
});

function withGame(element: React.ReactNode) {
    return render(
        <AuthContext.Provider value={authValue}>
            <MemoryRouter>{element}</MemoryRouter>
        </AuthContext.Provider>,
    );
}

describe('Game results', () => {
    it('gives the timed overlay initial focus and does not dismiss an active game with Escape', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post.mockResolvedValueOnce({
            data: {
                media_url: 'https://example.test/photo.jpg',
                server_time: new Date().toISOString(),
                view_expires_at: new Date(Date.now() + 2000).toISOString(),
                guess_expires_at: new Date(Date.now() + 122000).toISOString(),
            },
        });
        mocks.post.mockResolvedValueOnce({
            data: {
                view_expires_at: new Date(Date.now() + 2000).toISOString(),
                guess_expires_at: new Date(Date.now() + 122000).toISOString(),
                server_time: new Date().toISOString(),
            },
        });
        withGame(<Game gameMessage={message({ photo_id: 'focus-1', kind: 'challenge' })} onClose={vi.fn()} />);

        const dialog = await screen.findByRole('dialog', { name: 'Challenge photo' });
        await waitFor(() => expect(dialog).toHaveFocus());
        fireEvent.keyDown(dialog, { key: 'Escape' });
        expect(screen.getByRole('dialog', { name: 'Challenge photo' })).toBeInTheDocument();
    });

    it('shows how long each guess took next to its distance', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-8',
                group_id: 'group-1',
                actual_lat: 48,
                actual_long: 2,
                media_available: false,
                guesses: [
                    {
                        id: 'guess-1',
                        photo_id: 'photo-8',
                        user_id: 'user-1',
                        username: 'alice',
                        avatar: 'a.png',
                        score: 90,
                        distance: 100,
                        time_to_guess_ms: 45000,
                        elo_delta: 2,
                        created_at: new Date().toISOString(),
                    },
                    {
                        id: 'guess-2',
                        photo_id: 'photo-8',
                        user_id: 'user-2',
                        username: 'bob',
                        avatar: 'b.png',
                        score: 80,
                        distance: 5000,
                        time_to_guess_ms: 120000,
                        elo_delta: -2,
                        created_at: new Date().toISOString(),
                    },
                    // A legacy guess without a recorded view window omits the
                    // duration entirely: only the distance is shown.
                    {
                        id: 'guess-3',
                        photo_id: 'photo-8',
                        user_id: 'user-3',
                        username: 'carol',
                        avatar: 'c.png',
                        score: 70,
                        distance: 3000,
                        elo_delta: 0,
                        created_at: new Date().toISOString(),
                    },
                ],
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-8', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );
        // Under a minute shows only seconds; whole minutes drop the zero
        // seconds; both units appear otherwise (see the hidden-location test).
        expect(await screen.findByText('0.1 km away in 45sec')).toBeInTheDocument();
        expect(screen.getByText('5.0 km away in 2min')).toBeInTheDocument();
        const carolRow = screen.getByText('carol').closest('.score-card') as HTMLElement;
        expect(carolRow).toHaveTextContent('3.0 km away');
        expect(carolRow).not.toHaveTextContent(/ in /);
    });
});
