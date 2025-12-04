import { useState, useEffect } from 'react';
import Map from './Map';
import api from '../api';
import './Game.css';

interface GameProps {
    gameMessage: { group_id: string; user_id: string; content: string } | null;
    onGameComplete?: () => void;
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

export default function Game({ gameMessage, onGameComplete }: GameProps) {
    const [state, setState] = useState<GameState>({ status: 'IDLE' });
    const [selectedLocation, setSelectedLocation] = useState<{ lat: number; long: number } | null>(null);
    const currentUserId = JSON.parse(localStorage.getItem('user') || '{}').id;

    // Listen for game messages from parent
    useEffect(() => {
        if (!gameMessage) return;

        const msg = gameMessage;
        if (msg.content && msg.content.startsWith('NEW_PHOTO:')) {
            const parts = msg.content.split(':');
            const photoUserId = msg.user_id;

            // Skip if it's the user's own photo
            if (photoUserId === currentUserId) {
                console.log('Skipping own photo');
                setState({ status: 'SKIPPED' });
                setTimeout(() => setState({ status: 'IDLE' }), 2000);
                return;
            }

            startRound({ id: parts[1], url: parts[2], userId: photoUserId });
        }
    }, [gameMessage, currentUserId]);

    const startRound = (photo: any) => {
        setState({
            status: 'VIEWING_PHOTO',
            photoUrl: photo.url,
            photoId: photo.id,
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
            const res = await api.post('/guess', {
                photo_id: state.photoId,
                lat: selectedLocation.lat,
                long: selectedLocation.long,
            });

            setState((prev) => ({
                ...prev,
                status: 'RESULT',
                score: res.data.score,
                distance: res.data.distance,
                actualLocation: { lat: res.data.actual_lat, long: res.data.actual_long },
            }));
            onGameComplete?.();
        } catch (err: any) {
            console.error('Guess failed', err);
            if (err.response?.status === 400) {
                // Show error if trying to guess own photo
                alert('Cannot guess your own photo!');
                handleNextRound();
            }
        }
    };

    const handleNextRound = () => {
        setState({ status: 'IDLE' });
        setSelectedLocation(null);
    };

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

    const formatDistance = (meters: number) => {
        if (meters >= 1000) {
            return `${(meters / 1000).toFixed(2)} km`;
        }
        return `${Math.round(meters)} m`;
    };

    if (state.status === 'RESULT') {
        const isGoodScore = (state.score || 0) > 7000;
        return (
            <div className="game-overlay">
                <div className="result-view scale-in">
                    <div className={`result-header ${isGoodScore ? 'good-score' : ''}`}>
                        <div className="result-icon">{isGoodScore ? '🎉' : '📍'}</div>
                        <h2>{isGoodScore ? 'Great guess!' : 'Good try!'}</h2>
                        <div className="result-score">
                            <span className="score-value">{state.score}</span>
                            <span className="score-points">points</span>
                        </div>
                        <div className="result-distance">
                            {formatDistance(state.distance || 0)} away
                        </div>
                    </div>

                    <Map
                        onLocationSelect={() => { }}
                        selectedLocation={selectedLocation}
                        actualLocation={state.actualLocation}
                    />

                    <button onClick={handleNextRound} className="next-button btn btn-primary">
                        Next Round →
                    </button>

                    {isGoodScore && <div className="confetti-overlay"></div>}
                </div>
            </div>
        );
    }

    return null;
}
