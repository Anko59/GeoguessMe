import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { ChallengeAcceptance, ChallengeMediaDelivered, ChallengeResults, GuessResult, Message } from '../../types';
import Map from '../map/Map';
import Icon from '../ui/Icon';
import './Game.css';
import GuessScoreFeedback from './GuessScoreFeedback';
import { feedbackForScore } from './guessFeedback';

type Status =
    'idle' | 'accepting' | 'viewing' | 'waiting' | 'guessing' | 'submitting' | 'results' | 'expired' | 'error';
interface Position {
    lat: number;
    long: number;
}
interface GameState {
    status: Status;
    photoId?: string;
    mediaUrl?: string;
    mediaType?: string;
    deadline?: number;
    serverOffset: number;
    results?: ChallengeResults;
    message?: string;
}

interface GameProps {
    gameMessage: Message | null;
    onChallengeStatusChange?: (photoId: string, status: NonNullable<Message['challenge_status']>) => void;
    onClose: () => void;
}

export default function Game({ gameMessage, onChallengeStatusChange, onClose }: GameProps) {
    const { user } = useAuth();
    const [state, setState] = useState<GameState>({ status: 'idle', serverOffset: 0 });
    const [selectedLocation, setSelectedLocation] = useState<Position | null>(null);
    const [clock, setClock] = useState(() => Date.now());
    const [loadingMedia, setLoadingMedia] = useState(false);
    const [expandedResultImage, setExpandedResultImage] = useState<string | null>(null);
    const [guessFeedback, setGuessFeedback] = useState<ReturnType<typeof feedbackForScore> | null>(null);
    const [guessFeedbackScore, setGuessFeedbackScore] = useState<number | null>(null);

    const remaining = useMemo(
        () => (state.deadline ? Math.max(0, Math.ceil((state.deadline - (clock + state.serverOffset)) / 1000)) : 0),
        [clock, state.deadline, state.serverOffset],
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
            setState({ status: 'accepting', photoId, serverOffset: 0 });
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
                    setState({
                        status: 'viewing',
                        photoId,
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
                        URL.revokeObjectURL(mediaUrl);
                        setState({
                            status: 'error',
                            photoId,
                            serverOffset: 0,
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
                    setState({ status: 'guessing', photoId, deadline: serverDeadline, serverOffset });
                    return;
                }
                setState({
                    status: 'error',
                    photoId,
                    serverOffset: 0,
                    message: getAPIErrorMessage(mediaError, 'This challenge is no longer available.'),
                });
            } catch (requestError: unknown) {
                setState({
                    status: 'error',
                    photoId,
                    serverOffset: 0,
                    message: getAPIErrorMessage(requestError, 'This challenge is no longer available.'),
                });
            }
        },
        [loadMedia, onChallengeStatusChange],
    );

    const loadResults = useCallback(
        async (photoId: string, showError = true): Promise<boolean> => {
            setState({ status: 'accepting', photoId, serverOffset: 0 });
            try {
                const response = await api.get<ChallengeResults>(`/challenges/${photoId}/results`);
                const results = response.data;
                let mediaUrl: string | undefined;
                if (results.media_available && results.media_url) mediaUrl = await loadMedia(results.media_url);
                setState({
                    status: 'results',
                    photoId,
                    mediaUrl,
                    mediaType: results.media_type ?? undefined,
                    serverOffset: Date.parse(results.server_time) - Date.now(),
                    results,
                });
                onChallengeStatusChange?.(photoId, 'results');
                return true;
            } catch (requestError: unknown) {
                if (showError) {
                    setState({
                        status: 'error',
                        photoId,
                        serverOffset: 0,
                        message: getAPIErrorMessage(requestError, 'Results are not available yet.'),
                    });
                }
                return false;
            }
        },
        [loadMedia, onChallengeStatusChange],
    );

    const submitGuess = useCallback(async (): Promise<void> => {
        if (!state.photoId || !selectedLocation) return;
        setState((current) => ({ ...current, status: 'submitting' }));
        try {
            const response = await api.post<GuessResult>(`/challenges/${state.photoId}/guess`, selectedLocation);
            if (!response.data.duplicate) {
                setGuessFeedbackScore(response.data.score);
                setGuessFeedback(feedbackForScore(response.data.score));
            }
            onChallengeStatusChange?.(state.photoId, 'guessed');
            await loadResults(state.photoId);
        } catch (requestError: unknown) {
            setState((current) => ({
                ...current,
                status: 'error',
                message: getAPIErrorMessage(requestError, 'Your guess could not be submitted.'),
            }));
        }
    }, [loadResults, onChallengeStatusChange, selectedLocation, state.photoId]);

    const close = useCallback((): void => {
        if (state.mediaUrl) URL.revokeObjectURL(state.mediaUrl);
        setState({ status: 'idle', serverOffset: 0 });
        setSelectedLocation(null);
        setGuessFeedback(null);
        setGuessFeedbackScore(null);
        onClose();
    }, [onClose, state.mediaUrl]);

    useEffect(() => {
        if (!state.deadline || !['viewing', 'waiting'].includes(state.status)) return undefined;
        const timer = window.setInterval(() => setClock(Date.now()), 200);
        return () => window.clearInterval(timer);
    }, [state.deadline, state.status]);

    useEffect(() => {
        if (state.status === 'viewing' && remaining <= 0) setState((current) => ({ ...current, status: 'waiting' }));
        if (state.status === 'waiting' && remaining <= 0) setState((current) => ({ ...current, status: 'guessing' }));
    }, [remaining, state.status]);

    useEffect(() => {
        const photoId = gameMessage?.photo_id;
        if (!gameMessage || !photoId || !user) {
            if (!gameMessage) {
                setState({ status: 'idle', serverOffset: 0 });
                setGuessFeedback(null);
                setGuessFeedbackScore(null);
            }
            return;
        }
        setSelectedLocation(null);
        setGuessFeedback(null);
        setGuessFeedbackScore(null);
        if (gameMessage.user_id === user.id) {
            void loadResults(photoId);
            return;
        }
        void (async () => {
            const resultsAvailable = await loadResults(photoId, false);
            if (!resultsAvailable) await acceptChallenge(photoId);
        })();
    }, [acceptChallenge, gameMessage, loadResults, user]);

    useEffect(() => {
        if (!expandedResultImage) return undefined;
        const closeOnEscape = (event: KeyboardEvent) => {
            if (event.key === 'Escape') setExpandedResultImage(null);
        };
        window.addEventListener('keydown', closeOnEscape);
        return () => window.removeEventListener('keydown', closeOnEscape);
    }, [expandedResultImage]);
    const feedbackOverlay = guessFeedback ? (
        <GuessScoreFeedback
            feedback={guessFeedback}
            score={guessFeedbackScore ?? 0}
            onDismiss={() => {
                setGuessFeedback(null);
                setGuessFeedbackScore(null);
            }}
        />
    ) : null;
    const renderWithFeedback = (content: ReactNode) => (
        <>
            {content}
            {feedbackOverlay}
        </>
    );
    if (state.status === 'idle') return null;
    if (state.status === 'accepting')
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="loading-container fade-in">
                    <div className="loading-spinner" />
                    <p>{loadingMedia ? 'Loading private photo…' : 'Loading challenge…'}</p>
                </div>
            </div>,
        );
    if (state.status === 'error' || state.status === 'expired')
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="result-view scale-in">
                    <h2>{state.status === 'expired' ? 'Challenge expired' : 'Challenge unavailable'}</h2>
                    <p>{state.message ?? 'This challenge is no longer available.'}</p>
                    <button className="next-button btn btn-primary" onClick={close}>
                        Close
                    </button>
                </div>
            </div>,
        );
    if (state.status === 'viewing')
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="photo-view scale-in">
                    {state.mediaType?.startsWith('video/') ? (
                        <video
                            src={state.mediaUrl}
                            className="game-photo"
                            controls
                            playsInline
                            aria-label="Challenge video"
                        />
                    ) : (
                        <img src={state.mediaUrl} alt="Challenge location" className="game-photo" />
                    )}
                    <div className="timer-overlay">
                        <div className="timer-container">
                            <img src="/timer_icon.png" alt="" className="timer-icon" />
                            <div className="timer-text">{remaining}</div>
                        </div>
                    </div>
                </div>
            </div>,
        );
    if (state.status === 'waiting')
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="skipped-message fade-in">
                    <img src="/timer_icon.png" alt="" className="skip-icon" />
                    <p>Photo hidden</p>
                    <p className="skip-subtitle">Guessing opens in {remaining} seconds.</p>
                </div>
            </div>,
        );
    if (state.status === 'guessing' || state.status === 'submitting')
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="guessing-view fade-in">
                    <div className="guessing-header">
                        <h3>Where was this taken?</h3>
                        <p>Tap the map to place your guess.</p>
                    </div>
                    <Map
                        onLocationSelect={(lat, long) => setSelectedLocation({ lat, long })}
                        selectedLocation={selectedLocation}
                    />
                    <button
                        onClick={() => void submitGuess()}
                        disabled={!selectedLocation || state.status === 'submitting'}
                        className="guess-button btn btn-primary"
                    >
                        {state.status === 'submitting' ? (
                            'Submitting…'
                        ) : selectedLocation ? (
                            <>
                                Submit guess <Icon name="check" />
                            </>
                        ) : (
                            'Select a location…'
                        )}
                    </button>
                </div>
            </div>,
        );
    if (expandedResultImage)
        return renderWithFeedback(
            <div className="game-image-dialog" role="dialog" aria-modal="true" aria-label="Challenge photo full screen">
                <button
                    type="button"
                    className="game-image-dialog-close"
                    onClick={() => setExpandedResultImage(null)}
                    aria-label="Close full-screen photo"
                >
                    <Icon name="close" />
                </button>
                <img
                    src={expandedResultImage}
                    alt="Challenge location full screen"
                    className="game-image-dialog-photo"
                />
            </div>,
        );
    if (state.status === 'results' && state.results)
        return renderWithFeedback(
            <div className="game-overlay">
                <div className="result-view scale-in">
                    <div className="result-header">
                        <h2>Challenge results</h2>
                        {state.results.guesses.length > 0 ? (
                            <p>
                                {state.results.guesses.length} submitted score
                                {state.results.guesses.length === 1 ? '' : 's'}
                            </p>
                        ) : (
                            <p>No guesses have been submitted yet.</p>
                        )}
                    </div>
                    <div className="result-content">
                        <div className="result-details">
                            {state.mediaUrl && state.mediaType?.startsWith('video/') && (
                                <video
                                    src={state.mediaUrl}
                                    className="result-image"
                                    controls
                                    playsInline
                                    aria-label="Challenge result video"
                                />
                            )}
                            {state.mediaUrl && !state.mediaType?.startsWith('video/') && (
                                <button
                                    type="button"
                                    className="result-image-button"
                                    onClick={() => setExpandedResultImage(state.mediaUrl ?? null)}
                                    aria-label="View challenge photo full screen"
                                >
                                    <img src={state.mediaUrl} alt="Challenge location" className="result-image" />
                                </button>
                            )}
                            {!state.results.media_available && (
                                <p className="result-notice">
                                    The original media has been removed; scores remain available.
                                </p>
                            )}
                            <div className="score-list" aria-label="Submitted scores">
                                {state.results.guesses.map((guess) => (
                                    <div
                                        key={guess.id}
                                        className={`score-card ${guess.user_id === user?.id ? 'current-player' : ''}`}
                                    >
                                        <div>
                                            <strong>{guess.user_id === user?.id ? 'You' : guess.username}</strong>
                                            {guess.distance !== undefined && (
                                                <span>{(guess.distance / 1000).toFixed(1)} km away</span>
                                            )}
                                        </div>
                                        <b>{guess.score} pts</b>
                                    </div>
                                ))}
                            </div>
                        </div>
                        <div className="result-map" aria-label="Challenge map">
                            {state.results.location_hidden && (
                                <div className="result-location-hidden" role="note">
                                    <strong>The poster hasn’t revealed this location yet</strong>
                                    <span>
                                        Only your own guess is shown on the map. The exact spot and everyone else’s
                                        guesses will appear here after 48 hours.
                                    </span>
                                </div>
                            )}
                            <Map
                                onLocationSelect={() => undefined}
                                selectedLocation={null}
                                actualLocation={
                                    state.results.actual_lat !== undefined &&
                                    state.results.actual_long !== undefined &&
                                    !state.results.location_hidden
                                        ? { lat: state.results.actual_lat, long: state.results.actual_long }
                                        : null
                                }
                                guesses={state.results.guesses}
                            />
                        </div>
                    </div>
                    <button onClick={close} className="next-button btn btn-primary">
                        Close
                    </button>
                </div>
            </div>,
        );
    return null;
}
