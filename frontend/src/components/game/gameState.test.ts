import { describe, expect, it } from 'vitest';
import type { ChallengeResults } from '../../types';
import {
    MAX_GUESS_SCORE,
    gameReducer,
    initialGameState,
    scoreMultiplier,
    type GameAction,
    type GameState,
    type GameStatus,
} from './gameState';

const results: ChallengeResults = {
    photo_id: 'photo-1',
    group_id: 'group-1',
    actual_lat: 48,
    actual_long: 2,
    media_available: false,
    server_time: '2026-01-01T00:00:00Z',
    guesses: [],
};

function at(status: GameStatus, overrides: Partial<GameState> = {}): GameState {
    return { status, serverOffset: 0, ...overrides };
}

/** Dispatch an action and return the resulting state. */
function reduce(state: GameState, action: GameAction): GameState {
    return gameReducer(state, action);
}

describe('gameReducer', () => {
    it('starts at idle', () => {
        expect(initialGameState).toEqual({ status: 'idle', serverOffset: 0 });
    });

    describe('legal transitions', () => {
        it('loading starts the accepting phase from any state and clears the map pin', () => {
            const fromIdle = reduce(initialGameState, { type: 'loading', photoId: 'photo-1' });
            expect(fromIdle).toEqual({ status: 'accepting', photoId: 'photo-1', serverOffset: 0 });

            const pinned = at('guessing', {
                photoId: 'photo-1',
                deadline: 1000,
                serverOffset: 5,
                selectedLocation: { lat: 48.8, long: 2.3 },
            });
            expect(reduce(pinned, { type: 'loading', photoId: 'photo-2' })).toEqual({
                status: 'accepting',
                photoId: 'photo-2',
                serverOffset: 0,
            });
        });

        it('media-ready moves accepting to viewing with the loaded window', () => {
            const state = reduce(initialGameState, { type: 'loading', photoId: 'photo-1' });
            const next = reduce(state, {
                type: 'media-ready',
                photoId: 'photo-1',
                mediaUrl: 'blob:viewing',
                mediaType: 'image/jpeg',
                deadline: 5000,
                guessDeadline: 125000,
                scoreGraceSeconds: 60,
                serverOffset: 10,
            });
            expect(next).toEqual({
                status: 'viewing',
                photoId: 'photo-1',
                mediaUrl: 'blob:viewing',
                mediaType: 'image/jpeg',
                deadline: 5000,
                guessDeadline: 125000,
                scoreGraceSeconds: 60,
                serverOffset: 10,
            });
        });

        it('media-unavailable moves accepting to guessing when the window already elapsed', () => {
            const state = reduce(initialGameState, { type: 'loading', photoId: 'photo-1' });
            const next = reduce(state, {
                type: 'media-unavailable',
                photoId: 'photo-1',
                deadline: 1000,
                guessDeadline: 121000,
                scoreGraceSeconds: 60,
                serverOffset: 5,
            });
            expect(next).toEqual({
                status: 'guessing',
                photoId: 'photo-1',
                deadline: 1000,
                guessDeadline: 121000,
                scoreGraceSeconds: 60,
                serverOffset: 5,
            });
        });

        it('failure actions move accepting to error with the message', () => {
            const cases: Array<[GameAction, string]> = [
                [{ type: 'accept-failed', photoId: 'photo-1', message: 'gone' }, 'gone'],
                [
                    { type: 'media-failed', photoId: 'photo-1', message: 'window could not be started' },
                    'window could not be started',
                ],
                [{ type: 'results-failed', photoId: 'photo-1', message: 'not available' }, 'not available'],
            ];
            for (const [action, message] of cases) {
                const state = reduce(initialGameState, { type: 'loading', photoId: 'photo-1' });
                expect(reduce(state, action)).toEqual({
                    status: 'error',
                    photoId: 'photo-1',
                    message,
                    serverOffset: 0,
                });
            }
        });

        it('results-ready moves accepting to results', () => {
            const state = reduce(initialGameState, { type: 'loading', photoId: 'photo-1' });
            const next = reduce(state, { type: 'results-ready', photoId: 'photo-1', serverOffset: 7, results });
            expect(next).toEqual({ status: 'results', photoId: 'photo-1', serverOffset: 7, results });
        });

        it('results-ready preserves the celebration overlay set before the results load', () => {
            const accepting = reduce(reduce(initialGameState, { type: 'loading', photoId: 'photo-1' }), {
                type: 'show-feedback',
                score: 4920,
            });
            expect(accepting.feedback).toBeDefined();
            const next = reduce(accepting, { type: 'results-ready', photoId: 'photo-1', serverOffset: 7, results });
            expect(next.feedback).toEqual(accepting.feedback);
        });

        it('view-expired moves viewing to waiting and preserves the media fields', () => {
            const viewing = at('viewing', {
                photoId: 'photo-1',
                mediaUrl: 'blob:viewing',
                deadline: 1000,
                serverOffset: 5,
            });
            const next = reduce(viewing, { type: 'view-expired' });
            expect(next).toEqual({ ...viewing, status: 'waiting' });
        });

        it('guess-now moves waiting to guessing and preserves the media fields', () => {
            const waiting = at('waiting', {
                photoId: 'photo-1',
                mediaUrl: 'blob:viewing',
                deadline: 0,
                guessDeadline: 120000,
                serverOffset: 5,
            });
            expect(reduce(waiting, { type: 'guess-now' })).toEqual({ ...waiting, status: 'guessing' });
        });

        it('guess-timeout moves guessing to missed when the server deadline elapses', () => {
            const guessing = at('guessing', { photoId: 'photo-1', deadline: 0, guessDeadline: 120000 });
            expect(reduce(guessing, { type: 'guess-timeout' })).toEqual({ ...guessing, status: 'missed' });
        });

        it('select-location pins the map on the guessing phase', () => {
            const guessing = at('guessing', { photoId: 'photo-1' });
            const next = reduce(guessing, { type: 'select-location', lat: 48.8, long: 2.3 });
            expect(next.status).toBe('guessing');
            expect(next.selectedLocation).toEqual({ lat: 48.8, long: 2.3 });
        });

        it('guess-start moves a pinned guessing phase to submitting', () => {
            const guessing = at('guessing', { photoId: 'photo-1', selectedLocation: { lat: 48.8, long: 2.3 } });
            expect(reduce(guessing, { type: 'guess-start' }).status).toBe('submitting');
        });

        it('loading after a guess moves submitting back to accepting for the results load', () => {
            const submitting = at('submitting', { photoId: 'photo-1' });
            expect(reduce(submitting, { type: 'loading', photoId: 'photo-1' })).toEqual({
                status: 'accepting',
                photoId: 'photo-1',
                serverOffset: 0,
            });
        });

        it('guess-failed moves submitting to error', () => {
            const submitting = at('submitting', { photoId: 'photo-1' });
            expect(reduce(submitting, { type: 'guess-failed', message: 'could not submit' })).toEqual({
                status: 'error',
                photoId: 'photo-1',
                message: 'could not submit',
                serverOffset: 0,
            });
        });

        it('close returns results, error, expired, and missed to idle', () => {
            for (const status of ['results', 'error', 'expired', 'missed'] as const) {
                const state = at(status, { photoId: 'photo-1', message: 'x' });
                expect(reduce(state, { type: 'close' })).toEqual({ status: 'idle', serverOffset: 0 });
            }
        });

        it('reset returns any state to idle', () => {
            for (const status of [
                'accepting',
                'viewing',
                'waiting',
                'guessing',
                'submitting',
                'results',
                'error',
                'expired',
                'missed',
            ] as const) {
                const state = at(status, { photoId: 'photo-1', serverOffset: 3 });
                expect(reduce(state, { type: 'reset' })).toEqual({ status: 'idle', serverOffset: 0 });
            }
        });

        it('show-feedback records the celebration tier derived from the score', () => {
            const submitting = at('submitting', { photoId: 'photo-1' });
            const next = reduce(submitting, { type: 'show-feedback', score: 4920 });
            expect(next.status).toBe('submitting');
            expect(next.feedback).toEqual({
                feedback: expect.objectContaining({ label: 'Masterstroke', tone: 'excellent' }),
                score: 4920,
                partyDoubled: false,
            });
        });

        it('show-feedback carries the Party Time doubling flag onto the card', () => {
            const submitting = at('submitting', { photoId: 'photo-1' });
            const next = reduce(submitting, { type: 'show-feedback', score: 9840, partyDoubled: true });
            expect(next.feedback?.partyDoubled).toBe(true);
        });

        it('clear-feedback dismisses the overlay from any status', () => {
            const withFeedback = at('results', {
                photoId: 'photo-1',
                feedback: { feedback: { label: 'x', subtitle: 'y', tone: 'miss' }, score: 100 },
            });
            expect(reduce(withFeedback, { type: 'clear-feedback' })).toEqual({ ...withFeedback, feedback: undefined });
        });

        it('loading clears any leftover overlay for the incoming challenge', () => {
            const results = at('results', {
                photoId: 'photo-1',
                feedback: { feedback: { label: 'x', subtitle: 'y', tone: 'miss' }, score: 100 },
            });
            expect(reduce(results, { type: 'loading', photoId: 'photo-2' }).feedback).toBeUndefined();
        });
    });

    describe('illegal transitions are rejected', () => {
        it('returns the same state object for every illegal (status, action) pair', () => {
            const cases: Array<[GameState, GameAction]> = [
                // Phase-only actions fired from the wrong phase.
                [
                    at('viewing'),
                    {
                        type: 'media-ready',
                        photoId: 'photo-1',
                        mediaUrl: 'blob:x',
                        deadline: 1000,
                        guessDeadline: 121000,
                        scoreGraceSeconds: 60,
                        serverOffset: 0,
                    },
                ],
                [
                    at('viewing'),
                    {
                        type: 'media-unavailable',
                        photoId: 'photo-1',
                        deadline: 1000,
                        guessDeadline: 121000,
                        scoreGraceSeconds: 60,
                        serverOffset: 0,
                    },
                ],
                [at('viewing'), { type: 'results-ready', photoId: 'photo-1', serverOffset: 0, results }],
                [at('accepting'), { type: 'view-expired' }],
                [at('waiting'), { type: 'view-expired' }],
                [at('guessing'), { type: 'guess-now' }],
                [at('viewing'), { type: 'guess-now' }],
                [at('viewing'), { type: 'guess-timeout' }],
                [at('waiting'), { type: 'guess-timeout' }],
                [at('submitting'), { type: 'guess-timeout' }],
                [at('results'), { type: 'guess-timeout' }],
                [at('missed'), { type: 'guess-start' }],
                [at('submitting'), { type: 'guess-start' }],
                [at('submitting'), { type: 'select-location', lat: 1, long: 2 }],
                [at('results'), { type: 'guess-start' }],
                [
                    at('error'),
                    {
                        type: 'media-ready',
                        photoId: 'photo-1',
                        mediaUrl: 'blob:x',
                        deadline: 1000,
                        guessDeadline: 121000,
                        scoreGraceSeconds: 60,
                        serverOffset: 0,
                    },
                ],
                // A guess cannot start without a map pin.
                [at('guessing', { photoId: 'photo-1' }), { type: 'guess-start' }],
                // A guess failure outside the submitting phase is ignored.
                [at('guessing'), { type: 'guess-failed', message: 'no' }],
                // Close is only legal from the terminal views.
                [at('viewing'), { type: 'close' }],
                [at('idle'), { type: 'close' }],
                // Phase-only actions are rejected from idle.
                [
                    at('idle'),
                    {
                        type: 'media-ready',
                        photoId: 'photo-1',
                        mediaUrl: 'blob:x',
                        deadline: 1000,
                        guessDeadline: 121000,
                        scoreGraceSeconds: 60,
                        serverOffset: 0,
                    },
                ],
                [at('idle'), { type: 'select-location', lat: 1, long: 2 }],
                [at('idle'), { type: 'view-expired' }],
                [at('idle'), { type: 'guess-timeout' }],
            ];
            for (const [state, action] of cases) {
                expect(reduce(state, action), JSON.stringify({ state, action })).toBe(state);
            }
        });

        it('rejects completions belonging to a different challenge', () => {
            const accepting = at('accepting', { photoId: 'photo-2' });
            const stale = reduce(accepting, {
                type: 'results-ready',
                photoId: 'photo-1',
                serverOffset: 0,
                results,
            });
            expect(stale).toBe(accepting);
        });
    });

    describe('score notice', () => {
        it('show-score-notice is overlay-only and legal while guessing', () => {
            const state = at('guessing', { scoreNotice: undefined });
            const next = reduce(state, { type: 'show-score-notice', notice: 'grace-ended' });
            expect(next).toEqual({ ...state, scoreNotice: 'grace-ended' });
        });

        it('show-score-notice is rejected outside the guessing phase', () => {
            const state = at('viewing');
            expect(reduce(state, { type: 'show-score-notice', notice: 'grace-ended' })).toBe(state);
        });

        it('clear-score-notice dismisses the notice from any phase', () => {
            const noticing = at('guessing', { scoreNotice: 'grace-ended' });
            expect(reduce(noticing, { type: 'clear-score-notice' })).toEqual(at('guessing'));
            expect(reduce(at('idle'), { type: 'clear-score-notice' })).toEqual(at('idle'));
        });

        it('terminal close and reset drop the notice with the challenge', () => {
            const noticing = at('results', { scoreNotice: 'grace-ended' });
            expect(reduce(noticing, { type: 'close' })).toEqual(initialGameState);
            expect(reduce(noticing, { type: 'reset' })).toEqual(initialGameState);
        });
    });
});

describe('scoreMultiplier', () => {
    // Test vectors mirror backend/internal/game/score_timed_test.go so the
    // answering display and the server never disagree about the decay.
    const windowSeconds = 300;
    const graceSeconds = 60;

    it('keeps the full score during the grace period', () => {
        expect(scoreMultiplier(0, windowSeconds, graceSeconds)).toBe(1);
        expect(scoreMultiplier(30, windowSeconds, graceSeconds)).toBe(1);
        expect(scoreMultiplier(59, windowSeconds, graceSeconds)).toBe(1);
    });

    it('decays linearly after the grace period towards the 20% floor', () => {
        // At 150s: 1 - 0.8 * 90/239 ≈ 0.6987 — spot-check the formula so a
        // display drift cannot hide behind a threshold-only assertion.
        expect(scoreMultiplier(150, windowSeconds, graceSeconds)).toBeCloseTo(0.698744769874477, 12);
        expect(scoreMultiplier(150, windowSeconds, graceSeconds)).toBeLessThan(1);
        expect(scoreMultiplier(299, windowSeconds, graceSeconds)).toBe(0.2);
    });

    it('reaches zero at the deadline', () => {
        expect(scoreMultiplier(300, windowSeconds, graceSeconds)).toBe(0);
        expect(scoreMultiplier(360, windowSeconds, graceSeconds)).toBe(0);
    });

    it('treats a missing window as no decay', () => {
        expect(scoreMultiplier(120, 0, graceSeconds)).toBe(1);
    });

    it('supports grace periods other than the current policy value', () => {
        expect(scoreMultiplier(10, 60, 30)).toBe(1);
        expect(scoreMultiplier(30, 60, 30)).toBe(1);
        expect(scoreMultiplier(59, 60, 30)).toBe(0.2);
        expect(scoreMultiplier(60, 60, 30)).toBe(0);
    });

    it('pairs with MAX_GUESS_SCORE the same way the backend scales 5000', () => {
        expect(Math.round(MAX_GUESS_SCORE * scoreMultiplier(299, windowSeconds, graceSeconds))).toBe(1000);
        expect(Math.round(MAX_GUESS_SCORE * scoreMultiplier(300, windowSeconds, graceSeconds))).toBe(0);
    });
});
