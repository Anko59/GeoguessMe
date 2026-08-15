import type { ReactNode } from 'react';
import Map from '../map/Map';
import Icon from '../ui/Icon';
import FullScreenImage from '../ui/FullScreenImage';
import type { GameState, GamePosition } from './gameState';

// locationRevealClause renders the remaining hide duration for a hidden
// challenge location from the API's location_reveals_at timestamp so the UI
// never hardcodes the configured reveal period.
function locationRevealClause(revealsAt: string, referenceMs: number): string {
    const hours = Math.round((Date.parse(revealsAt) - referenceMs) / 3_600_000);
    if (hours < 1) return 'in under an hour';
    if (hours === 1) return 'after 1 hour';
    return `after ${hours} hours`;
}

interface GameViewProps {
    state: GameState;
    /** True while a private media blob is being fetched for the viewing window. */
    loadingMedia: boolean;
    /** Seconds left in the current viewing/waiting phase. */
    remaining: number;
    /** The client clock offset by the server offset (used for reveal clauses). */
    serverNowMs: number;
    /** Optional score-feedback overlay rendered above the active phase. */
    feedback: ReactNode | null;
    currentUserId?: string;
    onSelectLocation: (position: GamePosition) => void;
    onSubmitGuess: () => void;
    onClose: () => void;
}

function GameOverlay({ children, label }: { children: ReactNode; label: string }) {
    return (
        <div className="game-overlay" role="dialog" aria-modal="true" aria-label={label}>
            {children}
        </div>
    );
}

function GameLoadingView({ loadingMedia }: { loadingMedia: boolean }) {
    return (
        <GameOverlay label="Loading challenge">
            <div className="loading-container fade-in">
                <div className="loading-spinner" />
                <p>{loadingMedia ? 'Loading private photo…' : 'Loading challenge…'}</p>
            </div>
        </GameOverlay>
    );
}

function GameErrorView({ state, onClose }: { state: GameState; onClose: () => void }) {
    return (
        <GameOverlay label="Challenge unavailable">
            <div className="result-view scale-in">
                <h2>{state.status === 'expired' ? 'Challenge expired' : 'Challenge unavailable'}</h2>
                <p>{state.message ?? 'This challenge is no longer available.'}</p>
                <button className="next-button btn btn-primary" onClick={onClose}>
                    Close
                </button>
            </div>
        </GameOverlay>
    );
}

function GameViewingView({ state, remaining }: { state: GameState; remaining: number }) {
    return (
        <GameOverlay label="Challenge photo">
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
        </GameOverlay>
    );
}

function GameWaitingView({ remaining }: { remaining: number }) {
    return (
        <GameOverlay label="Challenge waiting">
            <div className="skipped-message fade-in">
                <img src="/timer_icon.png" alt="" className="skip-icon" />
                <p>Photo hidden</p>
                <p className="skip-subtitle">Guessing opens in {remaining} seconds.</p>
            </div>
        </GameOverlay>
    );
}

function GameGuessingView({
    state,
    onSelectLocation,
    onSubmitGuess,
}: {
    state: GameState;
    onSelectLocation: (position: GamePosition) => void;
    onSubmitGuess: () => void;
}) {
    const submitting = state.status === 'submitting';
    return (
        <GameOverlay label="Challenge guessing">
            <div className="guessing-view fade-in">
                <div className="guessing-header">
                    <h3>Where was this taken?</h3>
                    <p>Tap the map to place your guess.</p>
                </div>
                <Map
                    onLocationSelect={(lat, long) => onSelectLocation({ lat, long })}
                    selectedLocation={state.selectedLocation ?? null}
                />
                <button
                    onClick={onSubmitGuess}
                    disabled={!state.selectedLocation || submitting}
                    className="guess-button btn btn-primary"
                >
                    {submitting ? (
                        'Submitting…'
                    ) : state.selectedLocation ? (
                        <>
                            Submit guess <Icon name="check" />
                        </>
                    ) : (
                        'Select a location…'
                    )}
                </button>
            </div>
        </GameOverlay>
    );
}

function GameResultsView({
    state,
    currentUserId,
    serverNowMs,
    onClose,
}: {
    state: GameState;
    currentUserId?: string;
    serverNowMs: number;
    onClose: () => void;
}) {
    if (!state.results) return null;
    const revealClause =
        state.results.location_reveals_at === undefined
            ? 'when the reveal period ends'
            : locationRevealClause(state.results.location_reveals_at, serverNowMs);
    return (
        <GameOverlay label="Challenge results">
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
                            <FullScreenImage src={state.mediaUrl} alt="Challenge location">
                                <img src={state.mediaUrl} alt="Challenge location" className="result-image" />
                            </FullScreenImage>
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
                                    className={`score-card ${guess.user_id === currentUserId ? 'current-player' : ''}`}
                                >
                                    <div>
                                        <strong>{guess.user_id === currentUserId ? 'You' : guess.username}</strong>
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
                                    Only your own guess is shown on the map. The exact spot and everyone else’s guesses
                                    will appear here {revealClause}.
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
                <button onClick={onClose} className="next-button btn btn-primary">
                    Close
                </button>
            </div>
        </GameOverlay>
    );
}

/** The phase view for the current game state, plus the optional feedback
 *  overlay. Returns null while idle so the game renders nothing. */
export default function GameView({
    state,
    loadingMedia,
    remaining,
    serverNowMs,
    feedback,
    currentUserId,
    onSelectLocation,
    onSubmitGuess,
    onClose,
}: GameViewProps) {
    let view: ReactNode = null;
    switch (state.status) {
        case 'accepting':
            view = <GameLoadingView loadingMedia={loadingMedia} />;
            break;
        case 'viewing':
            view = <GameViewingView state={state} remaining={remaining} />;
            break;
        case 'waiting':
            view = <GameWaitingView remaining={remaining} />;
            break;
        case 'guessing':
        case 'submitting':
            view = <GameGuessingView state={state} onSelectLocation={onSelectLocation} onSubmitGuess={onSubmitGuess} />;
            break;
        case 'results':
            view = (
                <GameResultsView
                    state={state}
                    currentUserId={currentUserId}
                    serverNowMs={serverNowMs}
                    onClose={onClose}
                />
            );
            break;
        case 'error':
        case 'expired':
            view = <GameErrorView state={state} onClose={onClose} />;
            break;
        case 'idle':
            return null;
    }
    return (
        <>
            {view}
            {feedback}
        </>
    );
}
