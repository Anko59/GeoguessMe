import { useState, type FormEvent } from 'react';
import type { PublicGuessResult } from '../../types';
import Map from '../../components/map/Map';
import FeedDialog from './FeedDialog';
import FeedImage from './FeedImage';
import LocationPicker from './LocationPicker';
import { useFeedActions, usePublicResult } from './useFeed';

export default function FeedGame({
    id,
    alreadyResolved,
    restoreFocus,
    onClose,
    onResolved,
}: {
    id: string;
    alreadyResolved: boolean;
    restoreFocus: () => void;
    onClose: () => void;
    onResolved: () => void;
}) {
    const [point, setPoint] = useState<{ lat: number; long: number } | null>(null);
    const [submitted, setResult] = useState<PublicGuessResult | null>(null);
    const [viewingResult] = useState(alreadyResolved);
    const history = usePublicResult(id, viewingResult);
    const result = submitted ?? history.result;
    const { guess, pending, error } = useFeedActions(id);
    async function submit(event: FormEvent) {
        event.preventDefault();
        if (!point) return;
        const response = await guess(point);
        if (response) {
            setResult(response);
            onResolved();
        }
    }
    return (
        <FeedDialog
            title={result ? 'Place revealed' : 'Where in the world?'}
            onClose={onClose}
            busy={pending}
            restoreFocus={restoreFocus}
        >
            <div className="feed-game-photo">
                <FeedImage id={id} playing revealed />
            </div>
            {result ? (
                <>
                    <div className="feed-result" role="status">
                        <strong>{result.score.toLocaleString()} points</strong>
                        <span>
                            {result.distance < 1000
                                ? `${Math.round(result.distance)} m`
                                : `${(result.distance / 1000).toFixed(1)} km`}{' '}
                            from the spot
                        </span>
                        <p>The photo is now clear in your feed.</p>
                    </div>
                    <div className="feed-map">
                        <Map
                            selectedLocation={null}
                            actualLocation={{ lat: result.actual_lat, long: result.actual_long }}
                            guesses={[
                                {
                                    user_id: 'your-guess',
                                    lat: result.lat,
                                    long: result.long,
                                    username: 'Your guess',
                                    avatar: '',
                                    score: result.score,
                                },
                            ]}
                            onLocationSelect={() => {}}
                        />
                    </div>
                    <button className="btn btn-primary" onClick={onClose}>
                        Back to the feed
                    </button>
                </>
            ) : viewingResult ? (
                <div>
                    <p role="status">Loading your result…</p>
                    {history.error && (
                        <p role="alert">
                            {history.error}{' '}
                            <button disabled={history.pending} onClick={() => void history.load()}>
                                Retry result
                            </button>
                        </p>
                    )}
                </div>
            ) : (
                <form className="feed-form" onSubmit={submit}>
                    <p>Take a look around. You get one guess, with no time limit.</p>
                    <fieldset disabled={pending}>
                        <LocationPicker onChange={setPoint} />
                    </fieldset>
                    {error && (
                        <p className="error-message" role="alert">
                            {error}
                        </p>
                    )}
                    <button className="btn btn-primary" type="submit" disabled={pending || !point}>
                        {pending ? 'Checking your guess…' : 'Guess & reveal'}
                    </button>
                </form>
            )}
        </FeedDialog>
    );
}
