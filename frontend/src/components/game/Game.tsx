import { useCallback, useEffect, useMemo, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import { useAuth } from '../../context/AuthContext';
import type { ChallengeAcceptance, ChallengeResults, GuessResult, Message } from '../../types';
import Map from '../map/Map';
import './Game.css';

type Status = 'idle' | 'accepting' | 'viewing' | 'waiting' | 'guessing' | 'submitting' | 'results' | 'expired' | 'error';
interface Position { lat: number; long: number }
interface GameState { status: Status; photoId?: string; mediaUrl?: string; deadline?: number; serverOffset: number; results?: ChallengeResults; message?: string; }

interface GameProps { gameMessage: Message | null; onClose: () => void }

export default function Game({ gameMessage, onClose }: GameProps) {
    const { user } = useAuth();
    const [state, setState] = useState<GameState>({ status: 'idle', serverOffset: 0 });
    const [selectedLocation, setSelectedLocation] = useState<Position | null>(null);
    const [clock, setClock] = useState(() => Date.now());
    const [loadingMedia, setLoadingMedia] = useState(false);

    const remaining = useMemo(() => state.deadline ? Math.max(0, Math.ceil((state.deadline - (clock + state.serverOffset)) / 1000)) : 0, [clock, state.deadline, state.serverOffset]);

    const loadMedia = useCallback(async (url: string): Promise<string> => {
        if (url.startsWith('http://') || url.startsWith('https://')) return url;
        setLoadingMedia(true);
        try {
            const response = await api.get<Blob>(url, { responseType: 'blob' });
            return URL.createObjectURL(response.data);
        } finally { setLoadingMedia(false); }
    }, []);

    const acceptChallenge = useCallback(async (photoId: string): Promise<void> => {
        setState({ status: 'accepting', photoId, serverOffset: 0 });
        try {
            const response = await api.post<ChallengeAcceptance>(`/challenges/${photoId}/accept`);
            const data = response.data;
            const offset = Date.parse(data.server_time) - Date.now();
            const mediaUrl = await loadMedia(data.media_url);
            setState({ status: 'viewing', photoId, mediaUrl, deadline: Date.parse(data.view_expires_at), serverOffset: offset });
        } catch (requestError: unknown) { setState({ status: 'error', photoId, serverOffset: 0, message: getAPIErrorMessage(requestError, 'This challenge is no longer available.') }); }
    }, [loadMedia]);

    const loadResults = useCallback(async (photoId: string): Promise<void> => {
        setState({ status: 'accepting', photoId, serverOffset: 0 });
        try {
            const response = await api.get<ChallengeResults>(`/challenges/${photoId}/results`);
            const results = response.data;
            let mediaUrl: string | undefined;
            if (results.media_available && results.media_url) mediaUrl = await loadMedia(results.media_url);
            setState({ status: 'results', photoId, mediaUrl, serverOffset: Date.parse(results.server_time) - Date.now(), results });
        } catch (requestError: unknown) { setState({ status: 'error', photoId, serverOffset: 0, message: getAPIErrorMessage(requestError, 'Results are not available yet.') }); }
    }, [loadMedia]);

    const submitGuess = useCallback(async (): Promise<void> => {
        if (!state.photoId || !selectedLocation) return;
        setState((current) => ({ ...current, status: 'submitting' }));
        try {
            await api.post<GuessResult>(`/challenges/${state.photoId}/guess`, selectedLocation);
            await loadResults(state.photoId);
        } catch (requestError: unknown) { setState((current) => ({ ...current, status: 'error', message: getAPIErrorMessage(requestError, 'Your guess could not be submitted.') })); }
    }, [loadResults, selectedLocation, state.photoId]);

    const close = useCallback((): void => { if (state.mediaUrl) URL.revokeObjectURL(state.mediaUrl); setState({ status: 'idle', serverOffset: 0 }); setSelectedLocation(null); onClose(); }, [onClose, state.mediaUrl]);

    useEffect(() => { if (!state.deadline || !['viewing', 'waiting'].includes(state.status)) return undefined; const timer = window.setInterval(() => setClock(Date.now()), 200); return () => window.clearInterval(timer); }, [state.deadline, state.status]);

    useEffect(() => {
        if (state.status === 'viewing' && remaining <= 0) setState((current) => ({ ...current, status: 'waiting' }));
        if (state.status === 'waiting' && remaining <= 0) setState((current) => ({ ...current, status: 'guessing' }));
    }, [remaining, state.status]);

    useEffect(() => {
        if (!gameMessage || !gameMessage.photo_id || !user) { if (!gameMessage) setState({ status: 'idle', serverOffset: 0 }); return; }
        setSelectedLocation(null);
        if (gameMessage.user_id === user.id) void loadResults(gameMessage.photo_id); else void acceptChallenge(gameMessage.photo_id);
    }, [acceptChallenge, gameMessage, loadResults, user]);
    if (state.status === 'idle') return null;
    if (state.status === 'accepting') return <div className="game-overlay"><div className="loading-container fade-in"><div className="loading-spinner" /><p>{loadingMedia ? 'Loading private photo…' : 'Loading challenge…'}</p></div></div>;
    if (state.status === 'error' || state.status === 'expired') return <div className="game-overlay"><div className="result-view scale-in"><h2>{state.status === 'expired' ? 'Challenge expired' : 'Challenge unavailable'}</h2><p>{state.message ?? 'This challenge is no longer available.'}</p><button className="next-button btn btn-primary" onClick={close}>Close</button></div></div>;
    if (state.status === 'viewing') return <div className="game-overlay"><div className="photo-view scale-in"><img src={state.mediaUrl} alt="Challenge location" className="game-photo" /><div className="timer-overlay"><div className="timer-container"><img src="/timer_icon.png" alt="" className="timer-icon" /><div className="timer-text">{remaining}</div></div></div></div></div>;
    if (state.status === 'waiting') return <div className="game-overlay"><div className="skipped-message fade-in"><div className="skip-icon">⏱</div><p>Photo hidden</p><p className="skip-subtitle">Guessing opens in {remaining} seconds.</p></div></div>;
    if (state.status === 'guessing' || state.status === 'submitting') return <div className="game-overlay"><div className="guessing-view fade-in"><div className="guessing-header"><h3>Where was this taken?</h3><p>Tap the map to place your guess.</p></div><Map onLocationSelect={(lat, long) => setSelectedLocation({ lat, long })} selectedLocation={selectedLocation} /><button onClick={() => void submitGuess()} disabled={!selectedLocation || state.status === 'submitting'} className="guess-button btn btn-primary">{state.status === 'submitting' ? 'Submitting…' : selectedLocation ? 'Submit guess ✓' : 'Select a location…'}</button></div></div>;
    if (state.status === 'results' && state.results) return <div className="game-overlay"><div className="result-view scale-in"><h2>Challenge results</h2>{state.mediaUrl && <img src={state.mediaUrl} alt="Challenge location" className="result-image" />}{!state.results.media_available && <p>The original image has been removed; scores remain available.</p>}<div className="result-map"><Map onLocationSelect={() => undefined} selectedLocation={null} actualLocation={{ lat: state.results.actual_lat, long: state.results.actual_long }} guesses={state.results.guesses} /></div><button onClick={close} className="next-button btn btn-primary">Close</button></div></div>;
    return null;
}
