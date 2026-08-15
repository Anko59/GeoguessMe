import { describe, expect, it } from 'vitest';
import type { ChallengeResults } from '../../types';
import { gameReducer, initialGameState, type GameAction, type GameState, type GameStatus } from './gameState';

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
                serverOffset: 10,
            });
            expect(next).toEqual({
                status: 'viewing',
                photoId: 'photo-1',
                mediaUrl: 'blob:viewing',
                mediaType: 'image/jpeg',
                deadline: 5000,
                guessDeadline: 125000,
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
                serverOffset: 5,
            });
            expect(next).toEqual({
                status: 'guessing',
                photoId: 'photo-1',
                deadline: 1000,
                guessDeadline: 121000,
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
            });
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
});
