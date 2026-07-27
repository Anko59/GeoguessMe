import { fireEvent, render, screen } from '@testing-library/react';
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

describe('Game', () => {
    it('loads existing results, closes them, and handles unavailable challenges', async () => {
        const onClose = vi.fn();
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-1',
                group_id: 'group-1',
                actual_lat: 48,
                actual_long: 2,
                media_available: false,
                guesses: [],
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-1', kind: 'challenge' })}
                onClose={onClose}
            />,
        );
        expect(await screen.findByText('Challenge results')).toBeInTheDocument();
        expect(screen.getByText('The original media has been removed; scores remain available.')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(onClose).toHaveBeenCalled();

        mocks.get.mockRejectedValueOnce(new Error('not ready'));
        mocks.post.mockRejectedValueOnce(new Error('gone'));
        withGame(<Game gameMessage={message({ photo_id: 'photo-2', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByText('Challenge unavailable')).toBeInTheDocument();
        expect(screen.getByText('gone')).toBeInTheDocument();
    });

    it('accepts a challenge, selects a location, and submits a guess', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: 'https://example.test/photo.jpg',
                    server_time: new Date().toISOString(),
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                },
            })
            .mockResolvedValueOnce({ data: {} });
        withGame(<Game gameMessage={message({ photo_id: 'photo-3', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByAltText('Challenge location')).toBeInTheDocument();
    });

    it('opens a result photo full screen and closes it with Escape', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-4',
                group_id: 'group-1',
                actual_lat: 48,
                actual_long: 2,
                media_available: true,
                media_url: 'https://example.test/result.jpg',
                guesses: [],
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-4', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );

        fireEvent.click(await screen.findByRole('button', { name: 'View challenge photo full screen' }));
        expect(screen.getByRole('dialog', { name: 'Challenge photo full screen' })).toBeInTheDocument();
        expect(screen.getByAltText('Challenge location full screen')).toHaveAttribute(
            'src',
            'https://example.test/result.jpg',
        );
        fireEvent.keyDown(window, { key: 'Escape' });
        expect(screen.queryByRole('dialog', { name: 'Challenge photo full screen' })).not.toBeInTheDocument();
    });

    it('renders recorded video challenges with playback controls', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post.mockResolvedValueOnce({
            data: {
                media_url: 'https://example.test/challenge.webm',
                media_type: 'video/webm',
                server_time: new Date().toISOString(),
                view_expires_at: new Date(Date.now() + 2000).toISOString(),
            },
        });
        withGame(<Game gameMessage={message({ photo_id: 'video-1', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByLabelText('Challenge video')).toHaveAttribute(
            'src',
            'https://example.test/challenge.webm',
        );
    });
});
