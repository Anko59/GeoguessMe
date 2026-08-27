import { useCallback, useEffect, useMemo, useReducer, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { ChallengeAcceptance, ChallengeMediaDelivered, ChallengeResults, GuessResult, Message } from '../../types';
import { gameReducer, initialGameState } from './gameState';
import GameView from './GameViews';
import GuessScoreFeedback from './GuessScoreFeedback';
import './Game.css';

interface GameProps {
    gameMessage: Message | null;
    onChallengeStatusChange?: (photoId: string, status: NonNullable<Message['challenge_status']>) => void;
    onClose: () => void;
}

export default function Game({ gameMessage, onChallengeStatusChange, onClose }: GameProps) {
    const { user } = useAuth();
    const [state, dispatch] = useReducer(gameReducer, initialGameState);
    const [clock, setClock] = useState(() => Date.now());
    const [loadingMediaPhotoId, setLoadingMediaPhotoId] = useState<string>();
    const activePhotoIdRef = useRef(gameMessage?.photo_id);
    const mountedRef = useRef(true);

    useEffect(() => {
        activePhotoIdRef.current = gameMessage?.photo_id;
    }, [gameMessage?.photo_id]);

    useEffect(
        () => () => {
            mountedRef.current = false;
        },
        [],
    );

    const isCurrentPhoto = useCallback(
        (photoId: string, signal?: AbortSignal): boolean =>
            mountedRef.current && !signal?.aborted && activePhotoIdRef.current === photoId,
        [],
    );

    const remaining = useMemo(
        () => (state.deadline ? Math.max(0, Math.ceil((state.deadline - (clock + state.serverOffset)) / 1000)) : 0),
        [clock, state.deadline, state.serverOffset],
    );

    // The guessing-phase countdown derives from the server-published guess
    // deadline, so it survives app restarts: reopening a challenge whose
    // deadline already passed falls straight into the missed view.
    const guessRemaining = useMemo(
        () =>
            state.guessDeadline
                ? Math.max(0, Math.ceil((state.guessDeadline - (clock + state.serverOffset)) / 1000))
                : 0,
        [clock, state.guessDeadline, state.serverOffset],
    );

    // Full guess-window length (view end to guess deadline) for the timer
    // bar fill ratio; the window is the delta the API already publishes.
    const guessTotalSeconds = useMemo(() => {
        if (!state.guessDeadline || !state.deadline) return 0;
        return Math.max(0, Math.round((state.guessDeadline - state.deadline) / 1000));
    }, [state.deadline, state.guessDeadline]);

    // Object-URL lifecycle: the committed media URL has a single owner that
    // revokes it exactly once — when it is replaced or dropped by a state
    // transition, and when the component unmounts (previously leaked).
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

    const loadMedia = useCallback(
        async (url: string, photoId: string, signal?: AbortSignal): Promise<string> => {
            if (!isCurrentPhoto(photoId, signal)) throw new DOMException('Stale challenge operation', 'AbortError');
            if (url.startsWith('http://') || url.startsWith('https://')) return url;
            setLoadingMediaPhotoId(photoId);
            try {
                const apiPath = url.startsWith('/api/v1/') ? url.slice('/api/v1'.length) : url;
                const response = await api.get<Blob>(apiPath, { responseType: 'blob', signal });
                const objectUrl = URL.createObjectURL(response.data);
                if (!isCurrentPhoto(photoId, signal)) {
                    URL.revokeObjectURL(objectUrl);
                    throw new DOMException('Stale challenge operation', 'AbortError');
                }
                return objectUrl;
            } finally {
                if (isCurrentPhoto(photoId, signal)) setLoadingMediaPhotoId(undefined);
            }
        },
        [isCurrentPhoto],
    );

    const acceptChallenge = useCallback(
        async (photoId: string, signal?: AbortSignal): Promise<void> => {
            dispatch({ type: 'loading', photoId });
            try {
                const response = await api.post<ChallengeAcceptance>(`/challenges/${photoId}/accept`, undefined, {
                    signal,
                });
                if (!isCurrentPhoto(photoId, signal)) return;
                const data = response.data;
                const serverOffset = Date.parse(data.server_time) - Date.now();
                const serverDeadline = Date.parse(data.view_expires_at);
                const serverGuessDeadline = Date.parse(data.guess_expires_at);
                let mediaUrl: string | undefined;
                let mediaError: unknown;
                try {
                    mediaUrl = await loadMedia(data.media_url, photoId, signal);
                    const delivered = await api.post<ChallengeMediaDelivered>(
                        `/challenges/${photoId}/media-delivered`,
                        undefined,
                        { signal },
                    );
                    if (!isCurrentPhoto(photoId, signal)) {
                        if (mediaUrl.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                        return;
                    }
                    dispatch({
                        type: 'media-ready',
                        photoId,
                        mediaUrl,
                        mediaType: data.media_type,
                        deadline: Date.parse(delivered.data.view_expires_at),
                        guessDeadline: Date.parse(delivered.data.guess_expires_at),
                        serverOffset: Date.parse(delivered.data.server_time) - Date.now(),
                    });
                    onChallengeStatusChange?.(photoId, 'accepted');
                    return;
                } catch (loadError: unknown) {
                    mediaError = loadError;
                    if (mediaUrl) {
                        // The blob URL never entered state; revoke it directly.
                        if (mediaUrl.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                        if (isCurrentPhoto(photoId, signal)) {
                            dispatch({
                                type: 'media-failed',
                                photoId,
                                message: getAPIErrorMessage(
                                    mediaError,
                                    'The viewing window could not be started. Reopen the challenge to try again.',
                                ),
                            });
                        }
                        return;
                    }
                }
                if (!isCurrentPhoto(photoId, signal)) return;
                // The media could not be loaded. If the viewing window has
                // already elapsed (e.g. the player already viewed this
                // challenge), they may still submit a guess.
                if (serverDeadline <= Date.now() + serverOffset) {
                    dispatch({
                        type: 'media-unavailable',
                        photoId,
                        deadline: serverDeadline,
                        guessDeadline: serverGuessDeadline,
                        serverOffset,
                    });
                    return;
                }
                dispatch({
                    type: 'accept-failed',
                    photoId,
                    message: getAPIErrorMessage(mediaError, 'This challenge is no longer available.'),
                });
            } catch (requestError: unknown) {
                if (!isCurrentPhoto(photoId, signal)) return;
                dispatch({
                    type: 'accept-failed',
                    photoId,
                    message: getAPIErrorMessage(requestError, 'This challenge is no longer available.'),
                });
            }
        },
        [isCurrentPhoto, loadMedia, onChallengeStatusChange],
    );

    const loadResults = useCallback(
        async (photoId: string, showError = true, signal?: AbortSignal): Promise<boolean> => {
            dispatch({ type: 'loading', photoId });
            try {
                const response = await api.get<ChallengeResults>(`/challenges/${photoId}/results`, { signal });
                if (!isCurrentPhoto(photoId, signal)) return false;
                const results = response.data;
                let mediaUrl: string | undefined;
                if (results.media_available && results.media_url)
                    mediaUrl = await loadMedia(results.media_url, photoId, signal);
                if (!isCurrentPhoto(photoId, signal)) {
                    if (mediaUrl?.startsWith('blob:')) URL.revokeObjectURL(mediaUrl);
                    return false;
                }
                dispatch({
                    type: 'results-ready',
                    photoId,
                    mediaUrl,
                    mediaType: results.media_type ?? undefined,
                    serverOffset: Date.parse(results.server_time) - Date.now(),
                    results,
                });
                onChallengeStatusChange?.(photoId, 'results');
                return true;
            } catch (requestError: unknown) {
                if (showError && isCurrentPhoto(photoId, signal)) {
                    dispatch({
                        type: 'results-failed',
                        photoId,
                        message: getAPIErrorMessage(requestError, 'Results are not available yet.'),
                    });
                }
                return false;
            }
        },
        [isCurrentPhoto, loadMedia, onChallengeStatusChange],
    );

    const submitGuess = useCallback(async (): Promise<void> => {
        if (!state.selectedLocation || !state.photoId) return;
        const { photoId } = state;
        const guess = state.selectedLocation;
        dispatch({ type: 'guess-start' });
        try {
            const response = await api.post<GuessResult>(`/challenges/${photoId}/guess`, guess);
            // Start the results load first: its `loading` action clears the
            // overlay, so the celebration is dispatched afterwards and shows
            // during the loading phase and the results view.
            const resultsPromise = loadResults(photoId);
            if (!response.data.duplicate)
                dispatch({
                    type: 'show-feedback',
                    score: response.data.score,
                    partyDoubled: response.data.party_doubled === true,
                });
            onChallengeStatusChange?.(photoId, 'guessed');
            await resultsPromise;
        } catch (requestError: unknown) {
            dispatch({
                type: 'guess-failed',
                message: getAPIErrorMessage(requestError, 'Your guess could not be submitted.'),
            });
        }
    }, [loadResults, onChallengeStatusChange, state]);

    const close = useCallback((): void => {
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
        // The guess deadline is server-authoritative; once it passes, the
        // player cannot guess anymore. The server records a timed-out guess
        // (score 0) so the player appears in results afterwards.
        if (state.status === 'guessing' && state.guessDeadline !== undefined && guessRemaining <= 0) {
            const photoId = state.photoId;
            dispatch({ type: 'guess-timeout' });
            if (photoId) {
                void api.post(`/challenges/${photoId}/timeout`).catch(() => undefined);
            }
        }
    }, [guessRemaining, state.guessDeadline, state.photoId, state.status]);

    useEffect(() => {
        const photoId = gameMessage?.photo_id;
        if (!gameMessage || !photoId || !user) {
            if (!gameMessage) {
                dispatch({ type: 'reset' });
            }
            return;
        }
        // The `loading` action resets the celebration overlay and map pin for
        // the incoming challenge, exactly like the pre-refactor effect did.
        const controller = new AbortController();
        if (gameMessage.user_id === user.id) {
            void (async () => {
                await loadResults(photoId, true, controller.signal);
            })();
            return () => controller.abort();
        }
        void (async () => {
            const resultsAvailable = await loadResults(photoId, false, controller.signal);
            if (!resultsAvailable && isCurrentPhoto(photoId, controller.signal)) {
                await acceptChallenge(photoId, controller.signal);
            }
        })();
        return () => controller.abort();
    }, [acceptChallenge, gameMessage, isCurrentPhoto, loadResults, user]);

    const feedbackOverlay = state.feedback ? (
        <GuessScoreFeedback
            feedback={state.feedback.feedback}
            score={state.feedback.score}
            partyDoubled={state.feedback.partyDoubled}
            onDismiss={() => dispatch({ type: 'clear-feedback' })}
        />
    ) : null;

    return (
        <GameView
            state={state}
            loadingMedia={loadingMediaPhotoId === gameMessage?.photo_id}
            remaining={remaining}
            guessRemaining={guessRemaining}
            guessTotalSeconds={guessTotalSeconds}
            serverNowMs={clock + state.serverOffset}
            feedback={feedbackOverlay}
            currentUserId={user?.id}
            onSelectLocation={(position) =>
                dispatch({ type: 'select-location', lat: position.lat, long: position.long })
            }
            onSubmitGuess={() => void submitGuess()}
            onClose={close}
        />
    );
}
