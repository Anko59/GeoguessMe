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
        expect(screen.getByRole('dialog', { name: 'Challenge results' })).toHaveAttribute('aria-modal', 'true');
        expect(screen.getByText('The original media has been removed; scores remain available.')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(onClose).toHaveBeenCalled();

        mocks.get.mockRejectedValueOnce(new Error('not ready'));
        mocks.post.mockRejectedValueOnce(new Error('gone'));
        withGame(<Game gameMessage={message({ photo_id: 'photo-2', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByText('Challenge unavailable')).toBeInTheDocument();
        expect(screen.getByText('gone')).toBeInTheDocument();
    });

    it('starts the viewing window when the media finishes loading on a slow connection', async () => {
        // The results check fails; the accept response arrives with the server
        // window already elapsed, and the media blob finishes loading only now.
        const serverTime = Date.now();
        mocks.get
            .mockRejectedValueOnce(new Error('results not ready'))
            .mockResolvedValueOnce({ data: new Blob(['data'], { type: 'image/jpeg' }) });
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: '/api/v1/challenges/photo-6/media',
                    media_type: 'image/jpeg',
                    accepted_at: new Date(serverTime - 10000).toISOString(),
                    view_expires_at: new Date(serverTime).toISOString(),
                    guess_expires_at: new Date(serverTime + 120000).toISOString(),
                    server_time: new Date(serverTime).toISOString(),
                },
            })
            .mockResolvedValueOnce({
                data: {
                    view_expires_at: new Date(serverTime + 10000).toISOString(),
                    guess_expires_at: new Date(serverTime + 130000).toISOString(),
                    server_time: new Date(serverTime).toISOString(),
                },
            });
        const view = withGame(
            <Game gameMessage={message({ photo_id: 'photo-6', kind: 'challenge' })} onClose={vi.fn()} />,
        );

        // The image is shown even though the accept-time window has elapsed,
        // and the countdown reflects the full window from media-ready instead
        // of the consumed server window.
        expect(await screen.findByAltText('Challenge location')).toBeInTheDocument();
        expect(screen.getByText(/^[1-9]\d*$/)).toBeInTheDocument();
        // The pulsating timer icon is rendered above the displayed media.
        expect(view.container.querySelector('.timer-icon')).toHaveAttribute('src', '/timer_icon.png');
    });

    it('shows a notice instead of the location when the poster hid it', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-7',
                group_id: 'group-1',
                location_hidden: true,
                location_reveals_at: new Date(Date.now() + 48 * 3600 * 1000).toISOString(),
                guesses: [
                    {
                        id: 'guess-1',
                        photo_id: 'photo-7',
                        user_id: 'user-1',
                        username: 'alice',
                        avatar: 'a.png',
                        lat: 48.8,
                        long: 2.3,
                        score: 100,
                        distance: 1500,
                        elo_delta: 3,
                        created_at: new Date().toISOString(),
                    },
                    // Another player's guessed point and distance are omitted
                    // while the location is hidden: score only.
                    {
                        id: 'guess-2',
                        photo_id: 'photo-7',
                        user_id: 'user-2',
                        username: 'bob',
                        avatar: 'b.png',
                        score: 80,
                        elo_delta: -5,
                        created_at: new Date().toISOString(),
                    },
                ],
                media_available: false,
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-7', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );
        expect(await screen.findByText(/hasn’t revealed this location yet/)).toBeInTheDocument();
        expect(screen.getByText(/Only your own guess is shown on the map.*after 48 hours/)).toBeInTheDocument();
        // The viewer's own guess keeps its distance; the other player's row
        // shows only the score.
        expect(screen.getByText('1.5 km away')).toBeInTheDocument();
        expect(screen.queryByText(/km away/)).toBeInTheDocument();
        const bobRow = screen.getByText('bob').closest('.score-card') as HTMLElement;
        expect(bobRow).not.toHaveTextContent(/km away/);
        expect(bobRow).toHaveTextContent('80 pts');
        expect(screen.getByLabelText('Gained 3 Elo')).toHaveClass('elo-delta--gain');
        expect(screen.getByLabelText('Gained 3 Elo')).toHaveTextContent('+3 Elo');
        expect(screen.getByLabelText('Lost 5 Elo')).toHaveClass('elo-delta--loss');
        expect(screen.getByLabelText('Lost 5 Elo')).toHaveTextContent('-5 Elo');
    });

    it('derives the reveal duration from location_reveals_at instead of hardcoding it', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-7b',
                group_id: 'group-1',
                location_hidden: true,
                location_reveals_at: new Date(Date.now() + 24 * 3600 * 1000).toISOString(),
                guesses: [
                    {
                        id: 'guess-1',
                        photo_id: 'photo-7b',
                        user_id: 'user-1',
                        username: 'alice',
                        avatar: 'a.png',
                        score: 100,
                        created_at: new Date().toISOString(),
                    },
                ],
                media_available: false,
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-7b', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );
        expect(await screen.findByText(/hasn’t revealed this location yet/)).toBeInTheDocument();
        expect(screen.getByText(/will appear here after 24 hours/)).toBeInTheDocument();
    });

    it('uses singular grammar when the location reveals in one hour', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-7-hour',
                group_id: 'group-1',
                location_hidden: true,
                location_reveals_at: new Date(Date.now() + 3600 * 1000).toISOString(),
                guesses: [],
                media_available: false,
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-7-hour', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );
        expect(await screen.findByText(/will appear here after 1 hour(?!s)/)).toBeInTheDocument();
    });

    it('shows the hide notice without a duration when location_reveals_at is absent', async () => {
        mocks.get.mockResolvedValueOnce({
            data: {
                photo_id: 'photo-7c',
                group_id: 'group-1',
                location_hidden: true,
                guesses: [
                    {
                        id: 'guess-1',
                        photo_id: 'photo-7c',
                        user_id: 'user-1',
                        username: 'alice',
                        avatar: 'a.png',
                        score: 100,
                        created_at: new Date().toISOString(),
                    },
                ],
                media_available: false,
                server_time: new Date().toISOString(),
            },
        });
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-7c', kind: 'challenge' })}
                onClose={vi.fn()}
            />,
        );
        expect(await screen.findByText(/when the reveal period ends/)).toBeInTheDocument();
    });

    it('accepts a challenge, selects a location, and submits a guess', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: 'https://example.test/photo.jpg',
                    server_time: new Date().toISOString(),
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 122000).toISOString(),
                },
            })
            .mockResolvedValueOnce({
                data: {
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 122000).toISOString(),
                    server_time: new Date().toISOString(),
                },
            });
        withGame(<Game gameMessage={message({ photo_id: 'photo-3', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByAltText('Challenge location')).toBeInTheDocument();
        expect(screen.getByRole('dialog', { name: 'Challenge photo' })).toHaveAttribute('aria-modal', 'true');
    });

    it('celebrates a newly submitted top-tier score', async () => {
        // The results check fails, and the media fetch fails too (the player
        // already viewed this challenge, so the window has elapsed and they may
        // guess directly).
        mocks.get
            .mockRejectedValueOnce(new Error('results not ready'))
            .mockRejectedValueOnce(new Error('media expired'))
            .mockResolvedValueOnce({
                data: {
                    photo_id: 'photo-5',
                    group_id: 'group-1',
                    actual_lat: 48,
                    actual_long: 2,
                    media_available: false,
                    guesses: [],
                    server_time: new Date().toISOString(),
                },
            });
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: '/api/v1/challenges/photo-5/media',
                    media_type: 'image/jpeg',
                    accepted_at: new Date(Date.now() - 11000).toISOString(),
                    server_time: new Date().toISOString(),
                    view_expires_at: new Date(Date.now() - 1000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 119000).toISOString(),
                },
            })
            .mockResolvedValueOnce({
                data: {
                    guess_id: 'guess-5',
                    photo_id: 'photo-5',
                    score: 4920,
                    distance: 80,
                    created_at: new Date().toISOString(),
                    duplicate: false,
                },
            });
        withGame(<Game gameMessage={message({ photo_id: 'photo-5', kind: 'challenge' })} onClose={vi.fn()} />);

        fireEvent.click(await screen.findByRole('button', { name: 'Map' }));
        fireEvent.click(await screen.findByRole('button', { name: 'Submit guess' }));

        expect(await screen.findByRole('status')).toHaveTextContent('Masterstroke');
        expect(screen.getByRole('status')).toHaveTextContent('4,920 points');
        expect(mocks.post).toHaveBeenLastCalledWith('/challenges/photo-5/guess', { lat: 48.8, long: 2.3 });
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

        // The results photo opens in the shared full-screen dialog (same
        // component as chat attachments and avatars) and closes on Escape.
        fireEvent.click(await screen.findByRole('button', { name: 'View Challenge location full screen' }));
        const dialog = screen.getByRole('dialog', { name: 'Challenge location full screen' });
        expect(dialog.querySelector('img')).toHaveAttribute('src', 'https://example.test/result.jpg');
        fireEvent.keyDown(window, { key: 'Escape' });
        expect(screen.queryByRole('dialog', { name: 'Challenge location full screen' })).not.toBeInTheDocument();
    });

    it('revokes the challenge media object URL on unmount', async () => {
        const createSpy = vi.spyOn(URL, 'createObjectURL');
        const revokeSpy = vi.spyOn(URL, 'revokeObjectURL');
        mocks.get
            .mockRejectedValueOnce(new Error('results not ready'))
            .mockResolvedValueOnce({ data: new Blob(['data'], { type: 'image/jpeg' }) });
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: '/api/v1/challenges/photo-9/media',
                    media_type: 'image/jpeg',
                    accepted_at: new Date(Date.now() - 5000).toISOString(),
                    view_expires_at: new Date(Date.now() + 5000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 125000).toISOString(),
                    server_time: new Date().toISOString(),
                },
            })
            .mockResolvedValueOnce({
                data: {
                    view_expires_at: new Date(Date.now() + 5000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 125000).toISOString(),
                    server_time: new Date().toISOString(),
                },
            });
        const view = withGame(
            <Game gameMessage={message({ photo_id: 'photo-9', kind: 'challenge' })} onClose={vi.fn()} />,
        );
        await screen.findByAltText('Challenge location');

        const created = (createSpy.mock.results[0]?.value as string) ?? '';
        expect(created.startsWith('blob:')).toBe(true);
        view.unmount();
        expect(revokeSpy).toHaveBeenCalledTimes(1);
        expect(revokeSpy).toHaveBeenCalledWith(created);
    });

    it('revokes the results media object URL when the game closes', async () => {
        const createSpy = vi.spyOn(URL, 'createObjectURL');
        const revokeSpy = vi.spyOn(URL, 'revokeObjectURL');
        mocks.get
            .mockResolvedValueOnce({
                data: {
                    photo_id: 'photo-10',
                    group_id: 'group-1',
                    actual_lat: 48,
                    actual_long: 2,
                    media_available: true,
                    media_url: '/api/v1/challenges/photo-10/media',
                    media_type: 'image/jpeg',
                    guesses: [],
                    server_time: new Date().toISOString(),
                },
            })
            .mockResolvedValueOnce({ data: new Blob(['data'], { type: 'image/jpeg' }) });
        const onClose = vi.fn();
        withGame(
            <Game
                gameMessage={message({ user_id: 'user-1', photo_id: 'photo-10', kind: 'challenge' })}
                onClose={onClose}
            />,
        );
        await screen.findByRole('button', { name: 'View Challenge location full screen' });

        const created = (createSpy.mock.results[0]?.value as string) ?? '';
        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(onClose).toHaveBeenCalled();
        // The media URL is revoked exactly once when the state transition drops it.
        expect(revokeSpy).toHaveBeenCalledTimes(1);
        expect(revokeSpy).toHaveBeenCalledWith(created);
    });

    it('renders recorded video challenges with playback controls', async () => {
        mocks.get.mockRejectedValueOnce(new Error('results not ready'));
        mocks.post
            .mockResolvedValueOnce({
                data: {
                    media_url: 'https://example.test/challenge.webm',
                    media_type: 'video/webm',
                    server_time: new Date().toISOString(),
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 122000).toISOString(),
                },
            })
            .mockResolvedValueOnce({
                data: {
                    view_expires_at: new Date(Date.now() + 2000).toISOString(),
                    guess_expires_at: new Date(Date.now() + 122000).toISOString(),
                    server_time: new Date().toISOString(),
                },
            });
        withGame(<Game gameMessage={message({ photo_id: 'video-1', kind: 'challenge' })} onClose={vi.fn()} />);
        expect(await screen.findByLabelText('Challenge video')).toHaveAttribute(
            'src',
            'https://example.test/challenge.webm',
        );
    });
});
