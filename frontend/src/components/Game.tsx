import { useState, useEffect, useRef } from 'react';
import { getCurrentUserId } from '../utils/userUtils';
import Map from './Map';
import api from '../api';
import './Game.css';

interface GameProps {
    gameMessage: { group_id: string; user_id: string; content: string } | null;
    onGameComplete?: () => void;
    myGuesses: string[];
    onClose?: () => void;
}

interface GameState {
    status: 'IDLE' | 'VIEWING_PHOTO' | 'GUESSING' | 'RESULT' | 'SKIPPED';
    photoUrl?: string;
    photoId?: string;
    photoUserId?: string;
    timeLeft?: number;
    score?: number;
    distance?: number;
    actualLocation?: { lat: number; long: number };
}

export default function Game({ gameMessage, onGameComplete, myGuesses, onClose }: GameProps) {
    const [state, setState] = useState<GameState>({ status: 'IDLE' });
    const [selectedLocation, setSelectedLocation] = useState<{ lat: number; long: number } | null>(null);
    const [viewMode, setViewMode] = useState<'play' | 'view'>('play');
    const [photoData, setPhotoData] = useState<any>(null);
    const currentUserId = getCurrentUserId();
    const currentPhotoIdRef = useRef<string | null>(null);

    // Listen for game messages from parent
    useEffect(() => {
        if (!gameMessage) return;

        const msg = gameMessage;
        if (msg.content && msg.content.startsWith('NEW_PHOTO:')) {
            const parts = msg.content.split(':');
            const photoUserId = msg.user_id;
            const photoId = parts[1];
            const photoUrl = parts[2];

            // Skip if we're already displaying this photo (prevents flicker on myGuesses update)
            if (currentPhotoIdRef.current === photoId && (viewMode === 'view' || state.status !== 'IDLE')) {
                return;
            }

            // Track this photo
            currentPhotoIdRef.current = photoId;

            // Reset viewMode to play for any new message (defensive)
            // This prevents stale view state from previous challenges
            setViewMode('play');
            setPhotoData(null);

            // Check if this is view mode (sent by you OR already guessed)
            // We'll need to fetch guess data to show on map
            const isSentByMe = photoUserId === currentUserId;
            const isCompleted = myGuesses.includes(photoId);

            if (isSentByMe || isCompleted) {
                // View mode - show photo and where others guessed
                setViewMode('view');
                fetchPhotoDataForView(photoId, photoUrl);
                return;
            }

            // Skip if it's the user's own photo in play mode (double check)
            if (photoUserId === currentUserId) {
                console.log('Skipping own photo');
                setState({ status: 'SKIPPED' });
                setTimeout(() => {
                    setState({ status: 'IDLE' });
                    onClose?.();
                }, 2000);
                return;
            }

            // Normal play mode
            startRound({ id: photoId, url: photoUrl, userId: photoUserId });
        }
    }, [gameMessage, currentUserId, myGuesses, viewMode, state.status]);

    const startRound = (photo: any) => {
        setState({
            status: 'VIEWING_PHOTO',
            photoId: photo.id,
            photoUrl: photo.url,
            photoUserId: photo.userId,
            timeLeft: 10,
        });
    };

    useEffect(() => {
        let timer: any;
        if (state.status === 'VIEWING_PHOTO' && state.timeLeft && state.timeLeft > 0) {
            timer = setInterval(() => {
                setState((prev) => ({ ...prev, timeLeft: (prev.timeLeft || 0) - 1 }));
            }, 1000);
        } else if (state.status === 'VIEWING_PHOTO' && state.timeLeft === 0) {
            setState((prev) => ({ ...prev, status: 'GUESSING' }));
        }
        return () => clearInterval(timer);
    }, [state.status, state.timeLeft]);

    const handleGuess = async () => {
        if (!selectedLocation || !state.photoId) return;

        try {
            // Submit guess
            await api.post('/guess', {
                photo_id: state.photoId,
                lat: selectedLocation.lat,
                long: selectedLocation.long,
            });

            // Refresh parent (messages/guesses)
            onGameComplete?.();

            // Switch to view mode immediately
            setViewMode('view');

            // Fetch full details to show the result view
            await fetchPhotoDataForView(state.photoId, state.photoUrl!);

        } catch (err) {
            console.error('Failed to submit guess:', err);
        }
    };

    const handleNextRound = () => {
        setState({ status: 'IDLE' });
        setSelectedLocation(null);
        setViewMode('play');
        setPhotoData(null);
        currentPhotoIdRef.current = null;
        onClose?.();
    };

    const fetchPhotoDataForView = async (photoId: string, photoUrl: string) => {
        try {
            // Fetch the photo data to get actual location
            const res = await api.get(`/photo/details?id=${photoId}`);
            setPhotoData({
                id: photoId,
                url: photoUrl,
                lat: res.data.lat,
                long: res.data.long,
                guesses: res.data.guesses || [] // Assume backend returns guesses
            });
            setState({ status: 'RESULT' }); // Reuse result view for viewing
        } catch (err) {
            console.error('Failed to fetch photo data:', err);
            setState({ status: 'IDLE' });
        }
    };

    // View mode rendering
    if (viewMode === 'view' && state.status === 'RESULT') {
        // Show loading if photoData is not yet ready
        if (!photoData) {
            return (
                <div className="game-overlay">
                    <div className="loading-container fade-in">
                        <div className="loading-spinner"></div>
                        <p>Loading results...</p>
                    </div>
                </div>
            );
        }

        const myGuess = photoData.guesses?.find((g: any) => g.user_id === currentUserId);

        return (
            <div className="game-overlay">
                <div className="result-view scale-in">
                    <div className="result-header">
                        {/* Challenge Banner */}
                        <img
                            src="/challenge_banner.png"
                            alt="Challenge"
                            style={{
                                width: '100%',
                                maxWidth: '280px',
                                height: 'auto',
                                marginBottom: '1rem',
                                display: 'block',
                                margin: '0 auto 1rem auto'
                            }}
                        />

                        {myGuess && (
                            <div className="result-score">
                                <span className="score-value">{myGuess.score}</span>
                                <span className="score-points">points</span>
                            </div>
                        )}
                    </div>

                    <div className="view-mode-content" style={{ display: 'flex', flexDirection: 'column', gap: '1rem', height: '100%' }}>
                        <div className="view-photo-container" style={{ height: '200px', flexShrink: 0, borderRadius: '12px', overflow: 'hidden', border: '2px solid rgba(255,255,255,0.1)' }}>
                            <img src={photoData.url} alt="Challenge" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
                        </div>

                        <div style={{ flex: 1, minHeight: 0, borderRadius: '12px', overflow: 'hidden', border: '2px solid rgba(255,255,255,0.1)' }}>
                            <Map
                                onLocationSelect={() => { }}
                                selectedLocation={null}
                                actualLocation={{ lat: photoData.lat, long: photoData.long }}
                                guesses={photoData.guesses}
                            />
                        </div>
                    </div>

                    <button onClick={handleNextRound} className="next-button btn btn-primary" style={{ marginTop: '1rem' }}>
                        Close
                    </button>
                </div >
            </div >
        );
    }

    if (state.status === 'IDLE') {
        return null; // Hidden when idle
    }

    if (state.status === 'SKIPPED') {
        return (
            <div className="game-overlay">
                <div className="skipped-message fade-in">
                    <div className="skip-icon">📸</div>
                    <p>That's your photo!</p>
                    <p className="skip-subtitle">Waiting for other players to guess...</p>
                </div>
            </div>
        );
    }

    if (state.status === 'VIEWING_PHOTO') {
        return (
            <div className="game-overlay">
                <div className="photo-view scale-in">
                    <img src={state.photoUrl} alt="Guess location" className="game-photo" />
                    <div className="timer-overlay">
                        <div className="timer-container">
                            <img src="/timer_icon.png" alt="Timer" className="timer-icon" />
                            <div className="timer-text">{state.timeLeft}</div>
                        </div>
                    </div>
                </div>
            </div>
        );
    }

    if (state.status === 'GUESSING') {
        return (
            <div className="game-overlay">
                <div className="guessing-view fade-in">
                    <div className="guessing-header">
                        <h3>📍 Where was this taken?</h3>
                        <p>Tap on the map to place your guess</p>
                    </div>
                    <Map onLocationSelect={(lat, long) => setSelectedLocation({ lat, long })} selectedLocation={selectedLocation} />
                    <button
                        onClick={handleGuess}
                        disabled={!selectedLocation}
                        className="guess-button btn btn-primary"
                    >
                        {selectedLocation ? 'Submit Guess ✓' : 'Select a location...'}
                    </button>
                </div>
            </div>
        );
    }

    return null;
}
