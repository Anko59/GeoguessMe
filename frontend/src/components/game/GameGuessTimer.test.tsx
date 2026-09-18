import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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

const message = (photoId: string): Message => ({
    id: `message-${photoId}`,
    group_id: 'group-1',
    user_id: 'user-2',
    username: 'bob',
    avatar: 'avatar.png',
    kind: 'challenge',
    photo_id: photoId,
    created_at: '2026-01-01T00:00:00Z',
});

beforeEach(() => {
    vi.clearAllMocks();
    mocks.get.mockReset();
    mocks.post.mockReset();
    Element.prototype.scrollIntoView = vi.fn();
});

afterEach(() => vi.useRealTimers());

/** Guessing-phase entry through the media-unavailable path: the accept
 *  response carries the server windows relative to `elapsedSeconds` after
 *  the guess window opened, and the media refetch fails. */
function mockReopenedChallenge(photoId: string, elapsedSeconds: number, totalSeconds: number, graceSeconds = 60) {
    const now = Date.now();
    mocks.get.mockRejectedValueOnce(new Error('results not ready')).mockRejectedValueOnce(new Error('media expired'));
    mocks.post.mockResolvedValueOnce({
        data: {
            media_url: `/api/v1/challenges/${photoId}/media`,
            media_type: 'image/jpeg',
            accepted_at: new Date(now - (elapsedSeconds + 60) * 1000).toISOString(),
            server_time: new Date(now).toISOString(),
            view_expires_at: new Date(now - elapsedSeconds * 1000).toISOString(),
            guess_expires_at: new Date(now + (totalSeconds - elapsedSeconds) * 1000).toISOString(),
            score_grace_seconds: graceSeconds,
        },
    });
}

function withGame(photoId: string, onClose = vi.fn()) {
    return render(
        <AuthContext.Provider value={authValue}>
            <MemoryRouter>
                <Game gameMessage={message(photoId)} onClose={onClose} />
            </MemoryRouter>
        </AuthContext.Provider>,
    );
}

describe('Game guess timer', () => {
    it('shows the guess timer bar while the guessing phase is open', async () => {
        // The media cannot be fetched again (the player already viewed the
        // challenge), so the flow enters the guessing phase directly with the
        // server-published guess deadline still in the future.
        mocks.get
            .mockRejectedValueOnce(new Error('results not ready'))
            .mockRejectedValueOnce(new Error('media expired'));
        mocks.post.mockResolvedValueOnce({
            data: {
                media_url: '/api/v1/challenges/photo-11/media',
                media_type: 'image/jpeg',
                accepted_at: new Date(Date.now() - 11000).toISOString(),
                server_time: new Date().toISOString(),
                view_expires_at: new Date(Date.now() - 1000).toISOString(),
                guess_expires_at: new Date(Date.now() + 120000).toISOString(),
            },
        });
        withGame('photo-11');

        const timer = await screen.findByRole('timer');
        // The countdown reflects the full two-minute window left to guess and
        // the bar is rendered inside the guessing overlay.
        expect(timer.getAttribute('aria-label')).toMatch(/^Time left to guess: [12]:\d{2}$/);
        expect(timer.querySelector('.guess-timer-bar__fill')).toHaveAttribute(
            'style',
            expect.stringMatching(/width: [\d.]+%/),
        );
        expect(screen.getByRole('dialog', { name: 'Challenge guessing' })).toContainElement(timer);
    });

    it('shows full points during the published grace period', async () => {
        // Frozen clock: the displayed elapsed instant is exactly the anchor
        // the mock publishes, so the potential and countdown are exact.
        vi.useFakeTimers();
        mockReopenedChallenge('photo-21', 1, 300);
        withGame('photo-21');
        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
        });

        const timer = screen.getByRole('timer');
        expect(timer.getAttribute('aria-label')).toMatch(
            /^Full points available: 5,000 points\. Time left to guess: 4:59$/,
        );
        expect(timer.querySelector('.guess-timer-bar__score')).toHaveTextContent('5,000');
        expect(timer).not.toHaveClass('guess-timer-bar--decaying');
    });

    it('shows the decaying potential score after the grace period', async () => {
        // Reopening a challenge 150 seconds into a 300-second window: the
        // potential has decayed to roughly 70% and the bar is labeled
        // accordingly.
        vi.useFakeTimers();
        mockReopenedChallenge('photo-22', 150, 300);
        withGame('photo-22');
        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
        });

        const timer = screen.getByRole('timer');
        expect(timer.getAttribute('aria-label')).toContain('Potential score: 3,494 points');
        expect(timer.querySelector('.guess-timer-bar__score')).toHaveTextContent('3,494');
        expect(timer).toHaveClass('guess-timer-bar--decaying');
        // The grace-end notice is suppressed when the grace period ended
        // before the challenge was reopened: announcing an unavoidable decay
        // would only pressure the player.
        expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });

    it('warns about the zero-score cliff in the urgent state', async () => {
        vi.useFakeTimers();
        mockReopenedChallenge('photo-23', 286, 300);
        withGame('photo-23');

        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
        });
        const timer = screen.getByRole('timer');
        expect(timer).toHaveClass('guess-timer-bar--urgent');
        // 14 seconds before the deadline the potential is just above the
        // 20% floor: 1 - 0.8 * 226/239 → 1,218 points.
        expect(timer.querySelector('.guess-timer-bar__score')).toHaveTextContent('1,218');
        expect(timer.getAttribute('aria-label')).toContain('Guess now or the score drops to 0');
    });

    it('announces the grace-end transition once while guessing and dismisses it', async () => {
        vi.useFakeTimers();
        mockReopenedChallenge('photo-24', 55, 300);
        withGame('photo-24');
        // Flush the accept chain; the player is 55 seconds into the window,
        // still inside the 60-second full-points grace period.
        await act(async () => {
            await Promise.resolve();
            await Promise.resolve();
        });
        expect(screen.queryByRole('status')).not.toBeInTheDocument();

        // Crossing the grace boundary live announces the decay exactly once.
        await act(async () => {
            vi.advanceTimersByTime(60_000);
        });
        expect(screen.getByRole('status')).toHaveTextContent(/Full points have ended/i);
        await act(async () => {
            vi.advanceTimersByTime(1_000);
        });
        expect(screen.getByRole('status')).toHaveTextContent(/Full points have ended/i);

        // The notice owner auto-dismisses it shortly afterwards.
        await act(async () => {
            vi.advanceTimersByTime(4_500);
        });
        expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });

    it('marks the challenge as missed with 0 points when the guess deadline passes', async () => {
        // Reopening a challenge after its guess deadline (for example because
        // the app was closed): the server deadline is already in the past, so
        // the guessing phase times out immediately and the player sees the
        // missed view instead of being able to submit.
        mocks.get
            .mockRejectedValueOnce(new Error('results not ready'))
            .mockRejectedValueOnce(new Error('media expired'));
        mocks.post.mockResolvedValueOnce({
            data: {
                media_url: '/api/v1/challenges/photo-12/media',
                media_type: 'image/jpeg',
                accepted_at: new Date(Date.now() - 120000).toISOString(),
                server_time: new Date().toISOString(),
                view_expires_at: new Date(Date.now() - 1000).toISOString(),
                guess_expires_at: new Date(Date.now() - 500).toISOString(),
            },
        });
        const onClose = vi.fn();
        // The timeout endpoint is called when the deadline passes; allow it
        // to resolve so the missed view stays visible.
        mocks.post.mockResolvedValueOnce({ data: {} });
        withGame('photo-12', onClose);

        expect(await screen.findByText('You did not guess in time')).toBeInTheDocument();
        expect(screen.getByText('0 points')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Submit guess' })).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(onClose).toHaveBeenCalled();
        expect(screen.queryByText('You did not guess in time')).not.toBeInTheDocument();
    });
});
