import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react';
import { getAPIErrorMessage } from '../api';
import type { ChallengeResults, GuessResult } from '../types';
import {
    MAX_GUESS_SCORE,
    gameReducer,
    initialGameState,
    scoreMultiplier,
    type GamePosition,
    type GameState,
} from '../components/game/gameState';

/** The normalized timed-game window shared by group and feed challenges. */
export interface TimedGameWindow {
    mediaUrl?: string;
    mediaType?: string;
    viewExpiresAt: string;
    guessExpiresAt: string;
    scoreGraceSeconds: number;
    serverTime: string;
}

export type TimedGameMedia = { url: string; mediaType?: string } | { blob: Blob; mediaType?: string };

export interface TimedGameResults {
    results: ChallengeResults;
    media?: TimedGameMedia;
}

/**
 * A source adapter owns only the wire differences between challenge sources.
 * The phase machine, timer, object-URL lifecycle, feedback and stale-request
 * protection remain shared in this hook.
 */
export interface TimedGameAdapter {
    loadResults: (id: string, signal: AbortSignal) => Promise<TimedGameResults>;
    accept: (id: string, signal: AbortSignal) => Promise<TimedGameWindow>;
    loadMedia: (id: string, window: TimedGameWindow, signal: AbortSignal) => Promise<TimedGameMedia>;
    mediaDelivered: (id: string, signal: AbortSignal) => Promise<TimedGameWindow>;
    guess: (
        id: string,
        point: GamePosition,
        signal: AbortSignal,
    ) => Promise<
        Pick<GuessResult, 'score' | 'duplicate'> & {
            party_doubled?: boolean;
            timed_out?: boolean;
        }
    >;
    timeout: (id: string, signal: AbortSignal) => Promise<void>;
}

export interface UseTimedGameOptions {
    challengeId?: string;
    currentUserId?: string;
    /** Feed routes are already authenticated by their parent and may render
     * without a hydrated user object; group chat waits for auth hydration. */
    requiresCurrentUser?: boolean;
    isOwner: boolean;
    /** When true, the source is already resolved and should open results directly. */
    openResultsDirectly?: boolean;
    /** Group challenges retain their existing results-first fallback behavior. */
    checkResultsBeforeAccept?: boolean;
    adapter: TimedGameAdapter;
    onStatusChange?: (id: string, status: 'accepted' | 'guessed' | 'results') => void;
    onClose: () => void;
}

export interface UseTimedGameResult {
    state: GameState;
    loadingMedia: boolean;
    remaining: number;
    guessRemaining: number;
    guessTotalSeconds: number;
    potentialScore?: number;
    scoreNotice?: 'grace-ended';
    serverNowMs: number;
    selectLocation: (position: GamePosition) => void;
    submitGuess: () => void;
    dismissFeedback: () => void;
    close: () => void;
}

export function useTimedGame({
    challengeId,
    currentUserId,
    requiresCurrentUser = true,
    isOwner,
    openResultsDirectly = false,
    checkResultsBeforeAccept = true,
    adapter,
    onStatusChange,
    onClose,
}: UseTimedGameOptions): UseTimedGameResult {
    const [state, dispatch] = useReducer(gameReducer, initialGameState);
    const [clock, setClock] = useState(() => Date.now());
    const [loadingMedia, setLoadingMedia] = useState(false);
    const activeIdRef = useRef(challengeId);
    const mountedRef = useRef(true);

    useEffect(() => {
        activeIdRef.current = challengeId;
    }, [challengeId]);
    useEffect(
        () => () => {
            mountedRef.current = false;
        },
        [],
    );

    const isCurrent = useCallback(
        (id: string, signal?: AbortSignal) => mountedRef.current && !signal?.aborted && activeIdRef.current === id,
        [],
    );

    const remaining = useMemo(
        () => (state.deadline ? Math.max(0, Math.ceil((state.deadline - (clock + state.serverOffset)) / 1000)) : 0),
        [clock, state.deadline, state.serverOffset],
    );
    const guessRemaining = useMemo(
        () =>
            state.guessDeadline
                ? Math.max(0, Math.ceil((state.guessDeadline - (clock + state.serverOffset)) / 1000))
                : 0,
        [clock, state.guessDeadline, state.serverOffset],
    );
    const guessTotalSeconds = useMemo(() => {
        if (!state.guessDeadline || !state.deadline) return 0;
        return Math.max(0, Math.round((state.guessDeadline - state.deadline) / 1000));
    }, [state.deadline, state.guessDeadline]);
    const guessElapsedSeconds = useMemo(
        () =>
            state.deadline !== undefined && state.guessDeadline !== undefined
                ? Math.min(
                      guessTotalSeconds,
                      Math.max(0, Math.floor((clock + state.serverOffset - state.deadline) / 1000)),
                  )
                : 0,
        [clock, guessTotalSeconds, state.deadline, state.guessDeadline, state.serverOffset],
    );
    const potentialScore = useMemo(() => {
        const answering = state.status === 'guessing' || state.status === 'submitting';
        if (!answering || state.scoreGraceSeconds === undefined) return undefined;
        return Math.round(
            MAX_GUESS_SCORE * scoreMultiplier(guessElapsedSeconds, guessTotalSeconds, state.scoreGraceSeconds),
        );
    }, [guessElapsedSeconds, guessTotalSeconds, state.scoreGraceSeconds, state.status]);

    const graceNoticeRef = useRef<{ shown: boolean } | null>(null);
    useEffect(() => {
        const grace = state.scoreGraceSeconds;
        if (state.status !== 'guessing' || grace === undefined) {
            graceNoticeRef.current = null;
            return;
        }
        if (graceNoticeRef.current === null) {
            graceNoticeRef.current = { shown: guessElapsedSeconds >= grace };
            if (graceNoticeRef.current.shown) return;
        }
        if (!graceNoticeRef.current.shown && guessElapsedSeconds >= grace) {
            graceNoticeRef.current.shown = true;
            dispatch({ type: 'show-score-notice', notice: 'grace-ended' });
        }
    }, [guessElapsedSeconds, state.scoreGraceSeconds, state.status]);
    useEffect(() => {
        if (state.scoreNotice === undefined) return undefined;
        const timer = window.setTimeout(() => dispatch({ type: 'clear-score-notice' }), 4000);
        return () => window.clearTimeout(timer);
    }, [state.scoreNotice]);
    useEffect(() => {
        if (state.status !== 'guessing' && state.scoreNotice !== undefined) {
            dispatch({ type: 'clear-score-notice' });
        }
    }, [state.scoreNotice, state.status]);

    const mediaUrlRef = useRef<string | undefined>(undefined);
    useEffect(() => {
        const previous = mediaUrlRef.current;
        if (previous?.startsWith('blob:') && previous !== state.mediaUrl) URL.revokeObjectURL(previous);
        mediaUrlRef.current = state.mediaUrl;
    }, [state.mediaUrl]);
    useEffect(
        () => () => {
            const current = mediaUrlRef.current;
            if (current?.startsWith('blob:')) URL.revokeObjectURL(current);
        },
        [],
    );

    const materializeMedia = useCallback(
        async (id: string, media: TimedGameMedia, signal?: AbortSignal): Promise<string> => {
            const url = 'url' in media ? media.url : URL.createObjectURL(media.blob);
            if (!isCurrent(id, signal)) {
                if (url.startsWith('blob:')) URL.revokeObjectURL(url);
                throw new DOMException('Stale challenge operation', 'AbortError');
            }
            return url;
        },
        [isCurrent],
    );

    const loadResults = useCallback(
        async (id: string, showError = true, signal?: AbortSignal): Promise<boolean> => {
            dispatch({ type: 'loading', photoId: id });
            try {
                const loaded = await adapter.loadResults(id, signal ?? new AbortController().signal);
                let mediaUrl: string | undefined;
                if (loaded.media) mediaUrl = await materializeMedia(id, loaded.media, signal);
                if (!isCurrent(id, signal)) {
                    if (mediaUrl?.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                    return false;
                }
                dispatch({
                    type: 'results-ready',
                    photoId: id,
                    mediaUrl,
                    mediaType: loaded.media && 'mediaType' in loaded.media ? loaded.media.mediaType : undefined,
                    serverOffset: Date.parse(loaded.results.server_time) - Date.now(),
                    results: loaded.results,
                });
                onStatusChange?.(id, 'results');
                return true;
            } catch (requestError: unknown) {
                if (showError && isCurrent(id, signal)) {
                    dispatch({
                        type: 'results-failed',
                        photoId: id,
                        message: getAPIErrorMessage(requestError, 'Results are not available yet.'),
                    });
                }
                return false;
            }
        },
        [adapter, isCurrent, materializeMedia, onStatusChange],
    );

    const acceptChallenge = useCallback(
        async (id: string, signal?: AbortSignal): Promise<void> => {
            dispatch({ type: 'loading', photoId: id });
            try {
                const accepted = await adapter.accept(id, signal ?? new AbortController().signal);
                if (!isCurrent(id, signal)) return;
                const serverOffset = Date.parse(accepted.serverTime) - Date.now();
                const serverDeadline = Date.parse(accepted.viewExpiresAt);
                const serverGuessDeadline = Date.parse(accepted.guessExpiresAt);
                let mediaUrl: string | undefined;
                try {
                    setLoadingMedia(true);
                    const media = await adapter.loadMedia(id, accepted, signal ?? new AbortController().signal);
                    mediaUrl = await materializeMedia(id, media, signal);
                    const delivered = await adapter.mediaDelivered(id, signal ?? new AbortController().signal);
                    if (!isCurrent(id, signal)) {
                        if (mediaUrl.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                        return;
                    }
                    dispatch({
                        type: 'media-ready',
                        photoId: id,
                        mediaUrl,
                        mediaType: 'mediaType' in media ? media.mediaType : accepted.mediaType,
                        deadline: Date.parse(delivered.viewExpiresAt),
                        guessDeadline: Date.parse(delivered.guessExpiresAt),
                        scoreGraceSeconds: delivered.scoreGraceSeconds,
                        serverOffset: Date.parse(delivered.serverTime) - Date.now(),
                    });
                    onStatusChange?.(id, 'accepted');
                    return;
                } catch (loadError: unknown) {
                    if (mediaUrl?.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                    if (isCurrent(id, signal) && serverDeadline <= Date.now() + serverOffset) {
                        dispatch({
                            type: 'media-unavailable',
                            photoId: id,
                            deadline: serverDeadline,
                            guessDeadline: serverGuessDeadline,
                            scoreGraceSeconds: accepted.scoreGraceSeconds,
                            serverOffset,
                        });
                        return;
                    }
                    if (isCurrent(id, signal)) {
                        dispatch({
                            type: 'media-failed',
                            photoId: id,
                            message: getAPIErrorMessage(
                                loadError,
                                'The viewing window could not be started. Reopen the challenge to try again.',
                            ),
                        });
                    }
                    return;
                } finally {
                    if (isCurrent(id, signal)) setLoadingMedia(false);
                }
            } catch (requestError: unknown) {
                if (isCurrent(id, signal)) {
                    dispatch({
                        type: 'accept-failed',
                        photoId: id,
                        message: getAPIErrorMessage(requestError, 'This challenge is no longer available.'),
                    });
                }
            }
        },
        [adapter, isCurrent, materializeMedia, onStatusChange],
    );

    const submitGuess = useCallback(async () => {
        if (!state.selectedLocation || !state.photoId) return;
        const id = state.photoId;
        const guess = state.selectedLocation;
        dispatch({ type: 'guess-start' });
        try {
            const response = await adapter.guess(id, guess, new AbortController().signal);
            const resultsPromise = loadResults(id);
            if (!response.duplicate && !response.timed_out)
                dispatch({
                    type: 'show-feedback',
                    score: response.score,
                    partyDoubled: response.party_doubled === true,
                });
            onStatusChange?.(id, 'guessed');
            await resultsPromise;
        } catch (requestError: unknown) {
            dispatch({
                type: 'guess-failed',
                message: getAPIErrorMessage(requestError, 'Your guess could not be submitted.'),
            });
        }
    }, [adapter, loadResults, onStatusChange, state]);

    const close = useCallback(() => {
        dispatch({ type: 'close' });
        onClose();
    }, [onClose]);

    useEffect(() => {
        const viewingPhase = state.deadline !== undefined && ['viewing', 'waiting'].includes(state.status);
        const guessingPhase = state.status === 'guessing' && state.guessDeadline !== undefined;
        if (!viewingPhase && !guessingPhase) return undefined;
        const timer = window.setInterval(() => setClock(Date.now()), 200);
        return () => window.clearInterval(timer);
    }, [state.deadline, state.guessDeadline, state.status]);
    useEffect(() => {
        if (state.status === 'viewing' && remaining <= 0) dispatch({ type: 'view-expired' });
    }, [remaining, state.status]);
    useEffect(() => {
        if (state.status === 'waiting' && remaining <= 0) dispatch({ type: 'guess-now' });
    }, [remaining, state.status]);
    useEffect(() => {
        if (state.status === 'guessing' && state.guessDeadline !== undefined && guessRemaining <= 0) {
            const id = state.photoId;
            dispatch({ type: 'guess-timeout' });
            if (id) void adapter.timeout(id, new AbortController().signal).catch(() => undefined);
        }
    }, [adapter, guessRemaining, state.guessDeadline, state.photoId, state.status]);

    useEffect(() => {
        if (!challengeId || (requiresCurrentUser && !currentUserId)) {
            if (!challengeId) dispatch({ type: 'reset' });
            return;
        }
        const controller = new AbortController();
        if (openResultsDirectly || isOwner) {
            void loadResults(challengeId, true, controller.signal);
        } else if (checkResultsBeforeAccept) {
            void (async () => {
                const resultsAvailable = await loadResults(challengeId, false, controller.signal);
                if (!resultsAvailable && isCurrent(challengeId, controller.signal))
                    await acceptChallenge(challengeId, controller.signal);
            })();
        } else {
            void (async () => {
                await acceptChallenge(challengeId, controller.signal);
            })();
        }
        return () => controller.abort();
    }, [
        acceptChallenge,
        challengeId,
        checkResultsBeforeAccept,
        currentUserId,
        isCurrent,
        isOwner,
        loadResults,
        openResultsDirectly,
        requiresCurrentUser,
    ]);

    const selectLocation = useCallback((position: GamePosition) => {
        dispatch({ type: 'select-location', lat: position.lat, long: position.long });
    }, []);
    const dismissFeedback = useCallback(() => {
        dispatch({ type: 'clear-feedback' });
    }, []);

    return {
        state,
        loadingMedia,
        remaining,
        guessRemaining,
        guessTotalSeconds,
        potentialScore,
        scoreNotice: state.scoreNotice,
        serverNowMs: clock + state.serverOffset,
        selectLocation,
        submitGuess: () => void submitGuess(),
        dismissFeedback,
        close,
    };
}
