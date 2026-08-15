import type { ChallengeResults } from '../../types';
import { feedbackForScore, type GuessFeedback } from './guessFeedback';

/** The explicit game-flow states. `accepting` doubles as the generic loading
 *  phase for both challenge acceptance and results loading (matching the
 *  pre-refactor UI). `expired` is preserved as a display-only state that no
 *  transition produces today; it renders like `error` but with a distinct
 *  heading, so it stays in the union for parity. */
export type GameStatus =
    'idle' | 'accepting' | 'viewing' | 'waiting' | 'guessing' | 'submitting' | 'results' | 'expired' | 'error';

export interface GamePosition {
    lat: number;
    long: number;
}

/** The score-celebration overlay shown after a non-duplicate guess. */
export interface GameFeedback {
    feedback: GuessFeedback;
    score: number;
}

export interface GameState {
    status: GameStatus;
    photoId?: string;
    mediaUrl?: string;
    mediaType?: string;
    deadline?: number;
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
          serverOffset: number;
      }
    | { type: 'media-unavailable'; photoId: string; deadline: number; serverOffset: number }
    | { type: 'accept-failed'; photoId: string; message: string }
    | { type: 'media-failed'; photoId: string; message: string }
    | { type: 'view-expired' }
    | { type: 'guess-now' }
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
    | { type: 'show-feedback'; score: number }
    | { type: 'clear-feedback' }
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
 *   guessing              --select-location-->  guessing  (map pin updated)
 *   guessing (pinned)     --guess-start-->      submitting
 *   submitting            --loading-->          accepting (results load after a guess)
 *   submitting            --guess-failed-->     error
 *   results | error | expired --close-->        idle
 *   any                   --reset-->            idle      (challenge dismissed externally)
 *
 * `show-feedback` and `clear-feedback` are overlay-only actions: they mutate
 * `feedback` without changing the phase and are legal in every status.
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
                      serverOffset: action.serverOffset,
                  }
                : state;
        case 'media-unavailable':
            return state.status === 'accepting' && state.photoId === action.photoId
                ? {
                      status: 'guessing',
                      photoId: state.photoId,
                      deadline: action.deadline,
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
        case 'select-location':
            return state.status === 'guessing'
                ? { ...state, selectedLocation: { lat: action.lat, long: action.long } }
                : state;
        case 'show-feedback':
            return { ...state, feedback: { feedback: feedbackForScore(action.score), score: action.score } };
        case 'clear-feedback':
            return { ...state, feedback: undefined };
        case 'guess-start':
            return state.status === 'guessing' && state.selectedLocation ? withStatus(state, 'submitting') : state;
        case 'guess-failed':
            return state.status === 'submitting'
                ? { status: 'error', photoId: state.photoId, message: action.message, serverOffset: 0 }
                : state;
        case 'close':
            return state.status === 'results' || state.status === 'error' || state.status === 'expired'
                ? freshIdle()
                : state;
        case 'reset':
            return freshIdle();
    }
}
