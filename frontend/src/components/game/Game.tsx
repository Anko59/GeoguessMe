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
    const [loadingMedia, setLoadingMedia] = useState(false);

    const remaining = useMemo(
        () => (state.deadline ? Math.max(0, Math.ceil((state.deadline - (clock + state.serverOffset)) / 1000)) : 0),
        [clock, state.deadline, state.serverOffset],
    );

    // Object-URL lifecycle: the committed media URL has a single owner that
    // revokes it exactly once — when it is replaced or dropped by a state
    // transition, and when the component unmounts (previously leaked).
    const mediaUrlRef = useRef<string | undefined>(undefined);
    useEffect(() => {
        const previous = mediaUrlRef.current;
        if (previous && previous !== state.mediaUrl) URL.revokeObjectURL(previous);
        mediaUrlRef.current = state.mediaUrl;
    }, [state.mediaUrl]);
    useEffect(
        () => () => {
            const current = mediaUrlRef.current;
            if (current) URL.revokeObjectURL(current);
        },
        [],
    );

    const loadMedia = useCallback(async (url: string): Promise<string> => {
        if (url.startsWith('http://') || url.startsWith('https://')) return url;
        setLoadingMedia(true);
        try {
            const apiPath = url.startsWith('/api/v1/') ? url.slice('/api/v1'.length) : url;
            const response = await api.get<Blob>(apiPath, { responseType: 'blob' });
            return URL.createObjectURL(response.data);
        } finally {
            setLoadingMedia(false);
        }
    }, []);

    const acceptChallenge = useCallback(
        async (photoId: string): Promise<void> => {
            dispatch({ type: 'loading', photoId });
            try {
                const response = await api.post<ChallengeAcceptance>(`/challenges/${photoId}/accept`);
                const data = response.data;
                const serverOffset = Date.parse(data.server_time) - Date.now();
                const serverDeadline = Date.parse(data.view_expires_at);
                let mediaUrl: string | undefined;
                let mediaError: unknown;
                try {
                    mediaUrl = await loadMedia(data.media_url);
                    const delivered = await api.post<ChallengeMediaDelivered>(`/challenges/${photoId}/media-delivered`);
                    dispatch({
                        type: 'media-ready',
                        mediaUrl,
                        mediaType: data.media_type,
                        deadline: Date.parse(delivered.data.view_expires_at),
                        serverOffset: Date.parse(delivered.data.server_time) - Date.now(),
                    });
                    onChallengeStatusChange?.(photoId, 'accepted');
                    return;
                } catch (loadError: unknown) {
                    mediaError = loadError;
                    if (mediaUrl) {
                        // The blob URL never entered state; revoke it directly.
                        URL.revokeObjectURL(mediaUrl);
                        dispatch({
                            type: 'media-failed',
                            message: getAPIErrorMessage(
                                mediaError,
                                'The viewing window could not be started. Reopen the challenge to try again.',
                            ),
                        });
                        return;
                    }
                }
                // The media could not be loaded. If the viewing window has
                // already elapsed (e.g. the player already viewed this
                // challenge), they may still submit a guess.
                if (serverDeadline <= Date.now() + serverOffset) {
                    dispatch({ type: 'media-unavailable', deadline: serverDeadline, serverOffset });
                    return;
                }
                dispatch({
                    type: 'accept-failed',
                    message: getAPIErrorMessage(mediaError, 'This challenge is no longer available.'),
                });
            } catch (requestError: unknown) {
                dispatch({
                    type: 'accept-failed',
                    message: getAPIErrorMessage(requestError, 'This challenge is no longer available.'),
                });
            }
        },
        [loadMedia, onChallengeStatusChange],
    );

    const loadResults = useCallback(
        async (photoId: string, showError = true): Promise<boolean> => {
            dispatch({ type: 'loading', photoId });
            try {
                const response = await api.get<ChallengeResults>(`/challenges/${photoId}/results`);
                const results = response.data;
                let mediaUrl: string | undefined;
                if (results.media_available && results.media_url) mediaUrl = await loadMedia(results.media_url);
                dispatch({
                    type: 'results-ready',
                    mediaUrl,
                    mediaType: results.media_type ?? undefined,
                    serverOffset: Date.parse(results.server_time) - Date.now(),
                    results,
                });
                onChallengeStatusChange?.(photoId, 'results');
                return true;
            } catch (requestError: unknown) {
                if (showError) {
                    dispatch({
                        type: 'results-failed',
                        message: getAPIErrorMessage(requestError, 'Results are not available yet.'),
                    });
                }
                return false;
            }
        },
        [loadMedia, onChallengeStatusChange],
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
            if (!response.data.duplicate) dispatch({ type: 'show-feedback', score: response.data.score });
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
        if (!state.deadline || !['viewing', 'waiting'].includes(state.status)) return undefined;
        const timer = window.setInterval(() => setClock(Date.now()), 200);
        return () => window.clearInterval(timer);
    }, [state.deadline, state.status]);

    useEffect(() => {
        if (state.status === 'viewing' && remaining <= 0) dispatch({ type: 'view-expired' });
    }, [remaining, state.status]);

    useEffect(() => {
        if (state.status === 'waiting' && remaining <= 0) dispatch({ type: 'guess-now' });
    }, [remaining, state.status]);

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
        if (gameMessage.user_id === user.id) {
            void (async () => {
                await loadResults(photoId);
            })();
            return;
        }
        void (async () => {
            const resultsAvailable = await loadResults(photoId, false);
            if (!resultsAvailable) await acceptChallenge(photoId);
        })();
    }, [acceptChallenge, gameMessage, loadResults, user]);

    const feedbackOverlay = state.feedback ? (
        <GuessScoreFeedback
            feedback={state.feedback.feedback}
            score={state.feedback.score}
            onDismiss={() => dispatch({ type: 'clear-feedback' })}
        />
    ) : null;

    return (
        <GameView
            state={state}
            loadingMedia={loadingMedia}
            remaining={remaining}
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
