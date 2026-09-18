import type { ChallengeResults } from '../../types';
import { feedbackForScore, type GuessFeedback } from './guessFeedback';

/** The explicit game-flow states. `accepting` doubles as the generic loading
 *  phase for both challenge acceptance and results loading (matching the
 *  pre-refactor UI). `expired` is preserved as a display-only state that no
 *  transition produces today; it renders like `error` but with a distinct
 *  heading, so it stays in the union for parity. `missed` is the terminal
 *  state after the server-authoritative guess window elapses without a
 *  guess. */
export type GameStatus =
    | 'idle'
    | 'accepting'
    | 'viewing'
    | 'waiting'
    | 'guessing'
    | 'submitting'
    | 'results'
    | 'expired'
    | 'error'
    | 'missed';

export interface GamePosition {
    lat: number;
    long: number;
}

/** The score-celebration overlay shown after a non-duplicate guess. */
export interface GameFeedback {
    feedback: GuessFeedback;
    score: number;
    /** True when an active Party Time doubled this guess (wire
     *  `party_doubled`); drives the ×2 badge on the score card. */
    partyDoubled?: boolean;
}

export interface GameState {
    status: GameStatus;
    photoId?: string;
    mediaUrl?: string;
    mediaType?: string;
    /** End of the private viewing window (drives the photo countdown). */
    deadline?: number;
    /** Server-authoritative end of the guessing window; the map phase
     *  countdown and the timeout transition are derived from it. The timer
     *  therefore keeps running even when the app is closed. */
    guessDeadline?: number;
    /** Seconds after the guessing window opens with the full 5000-point
     *  maximum (wire `score_grace_seconds`); when published, the timer bar
     *  visualizes the score decay after this grace period. Optional so an
     *  older backend response simply hides the decay display. */
    scoreGraceSeconds?: number;
    /** Transient answering-phase notice (for example when the full-points
     *  grace period ends); auto-cleared by the owner, never persisted. */
    scoreNotice?: 'grace-ended';
    serverOffset: number;
    results?: ChallengeResults;
    message?: string;
    /** The map pin for the pending guess; cleared when a new challenge loads. */
    selectedLocation?: GamePosition;
    /** The score-celebration overlay; orthogonal to the phase machine and
     *  cleared on dismissal, close, reset, or a new challenge message. */
    feedback?: GameFeedback;
}

export type GameAction =
    | { type: 'loading'; photoId: string }
    | {
          type: 'media-ready';
          photoId: string;
          mediaUrl: string;
          mediaType?: string;
          deadline: number;
          guessDeadline: number;
          scoreGraceSeconds: number;
          serverOffset: number;
      }
    | {
          type: 'media-unavailable';
          photoId: string;
          deadline: number;
          guessDeadline: number;
          scoreGraceSeconds: number;
          serverOffset: number;
      }
    | { type: 'accept-failed'; photoId: string; message: string }
    | { type: 'media-failed'; photoId: string; message: string }
    | { type: 'view-expired' }
    | { type: 'guess-now' }
    | { type: 'guess-timeout' }
    | { type: 'select-location'; lat: number; long: number }
    | { type: 'guess-start' }
    | { type: 'guess-failed'; message: string }
    | {
          type: 'results-ready';
          photoId: string;
          mediaUrl?: string;
          mediaType?: string;
          serverOffset: number;
          results: ChallengeResults;
      }
    | { type: 'results-failed'; photoId: string; message: string }
    | { type: 'show-feedback'; score: number; partyDoubled?: boolean }
    | { type: 'clear-feedback' }
    | { type: 'show-score-notice'; notice: 'grace-ended' }
    | { type: 'clear-score-notice' }
    | { type: 'close' }
    | { type: 'reset' };

export const initialGameState: GameState = { status: 'idle', serverOffset: 0 };

const freshIdle = (): GameState => ({ status: 'idle', serverOffset: 0 });

/** Keep every field the target phase needs while changing only the status. */
const withStatus = (state: GameState, status: GameStatus): GameState => ({ ...state, status });

/**
 * Legal transitions for the challenge flow:
 *
 *   idle / any            --loading-->    accepting
 *   accepting             --media-ready-->    viewing
 *   accepting             --media-unavailable-->  guessing  (media gone, window elapsed)
 *   accepting             --accept-failed | media-failed-->  error
 *   accepting             --results-ready-->    results   (results loaded directly)
 *   accepting             --results-failed-->   error
 *   viewing               --view-expired-->     waiting
 *   waiting               --guess-now-->        guessing
 *   guessing              --guess-timeout-->    missed   (server guess deadline elapsed)
 *   guessing              --select-location-->  guessing  (map pin updated)
 *   guessing (pinned)     --guess-start-->      submitting
 *   submitting            --loading-->          accepting (results load after a guess)
 *   submitting            --guess-failed-->     error
 *   results | error | expired | missed --close-->        idle
 *   any                   --reset-->            idle      (challenge dismissed externally)
 *
 * `show-feedback` and `clear-feedback` are overlay-only actions: they mutate
 * `feedback` without changing the phase and are legal in every status.
 * `show-score-notice` is overlay-only and legal only while guessing (the
 * notice describes the live answering window); `clear-score-notice` is legal
 * everywhere so the notice owner can dismiss it from any phase.
 *
 * Every other (status, action) pair is an illegal transition and is rejected
 * by returning the current state unchanged; the transition tests pin both the
 * legal table above and the rejections.
 */
export function gameReducer(state: GameState, action: GameAction): GameState {
    switch (action.type) {
        case 'loading':
            // Any phase may start loading a challenge or its results; a fresh
            // load clears the previous map pin.
            return { status: 'accepting', photoId: action.photoId, serverOffset: 0 };
        case 'media-ready':
            return state.status === 'accepting' && state.photoId === action.photoId
                ? {
                      status: 'viewing',
                      photoId: state.photoId,
                      mediaUrl: action.mediaUrl,
                      mediaType: action.mediaType,
                      deadline: action.deadline,
                      guessDeadline: action.guessDeadline,
                      scoreGraceSeconds: action.scoreGraceSeconds,
                      serverOffset: action.serverOffset,
                  }
                : state;
        case 'media-unavailable':
            return state.status === 'accepting' && state.photoId === action.photoId
                ? {
                      status: 'guessing',
                      photoId: state.photoId,
                      deadline: action.deadline,
                      guessDeadline: action.guessDeadline,
                      scoreGraceSeconds: action.scoreGraceSeconds,
                      serverOffset: action.serverOffset,
                  }
                : state;
        case 'accept-failed':
        case 'media-failed':
        case 'results-failed':
            return state.status === 'accepting' && state.photoId === action.photoId
                ? { status: 'error', photoId: state.photoId, message: action.message, serverOffset: 0 }
                : state;
        case 'results-ready':
            return state.status === 'accepting' && state.photoId === action.photoId
                ? {
                      status: 'results',
                      photoId: state.photoId,
                      mediaUrl: action.mediaUrl,
                      mediaType: action.mediaType,
                      serverOffset: action.serverOffset,
                      results: action.results,
                      // Preserve the celebration overlay set just before the
                      // results load resolves.
                      feedback: state.feedback,
                  }
                : state;
        case 'view-expired':
            return state.status === 'viewing' ? withStatus(state, 'waiting') : state;
        case 'guess-now':
            return state.status === 'waiting' ? withStatus(state, 'guessing') : state;
        case 'guess-timeout':
            // The server deadline has elapsed without a submitted guess: the
            // challenge is lost (0 points) and the player must close the view.
            return state.status === 'guessing' ? withStatus(state, 'missed') : state;
        case 'select-location':
            return state.status === 'guessing'
                ? { ...state, selectedLocation: { lat: action.lat, long: action.long } }
                : state;
        case 'show-feedback':
            return {
                ...state,
                feedback: {
                    feedback: feedbackForScore(action.score),
                    score: action.score,
                    partyDoubled: action.partyDoubled ?? false,
                },
            };
        case 'clear-feedback':
            return { ...state, feedback: undefined };
        case 'show-score-notice':
            return state.status === 'guessing' ? { ...state, scoreNotice: action.notice } : state;
        case 'clear-score-notice':
            return state.scoreNotice === undefined ? state : { ...state, scoreNotice: undefined };
        case 'guess-start':
            return state.status === 'guessing' && state.selectedLocation ? withStatus(state, 'submitting') : state;
        case 'guess-failed':
            return state.status === 'submitting'
                ? { status: 'error', photoId: state.photoId, message: action.message, serverOffset: 0 }
                : state;
        case 'close':
            return state.status === 'results' ||
                state.status === 'error' ||
                state.status === 'expired' ||
                state.status === 'missed'
                ? freshIdle()
                : state;
        case 'reset':
            return freshIdle();
    }
}

/** Maximum score of a pinpoint guess; mirrors the backend scoring scale and
 *  is the base for the potential-score decay display. */
export const MAX_GUESS_SCORE = 5000;

/** Fraction of the maximum score still achievable `elapsedSeconds` after the
 *  guessing window opened. Mirrors backend/internal/game/score.go
 *  TimeMultiplier exactly (including the one-second anchor that reaches the
 *  0.2 floor one second before the deadline) so the answering UI can show the
 *  decay without re-deriving server policy. */
export function scoreMultiplier(elapsedSeconds: number, windowSeconds: number, graceSeconds: number): number {
    if (!(windowSeconds > 0)) return 1;
    const grace = Math.max(0, graceSeconds);
    if (elapsedSeconds < grace) return 1;
    if (elapsedSeconds >= windowSeconds) return 0;
    const penaltySpan = windowSeconds - grace - 1;
    if (penaltySpan <= 0) return 1;
    const offset = elapsedSeconds - grace;
    if (offset <= 0) return 1;
    if (offset >= penaltySpan) return 0.2;
    return 1 - 0.8 * (offset / penaltySpan);
}
