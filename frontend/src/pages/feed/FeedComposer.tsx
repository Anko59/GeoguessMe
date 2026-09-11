import { useEffect, useId, useState, type FormEvent } from 'react';
import { useFeedActions } from './useFeed';
import FeedDialog from './FeedDialog';
import LocationPicker from './LocationPicker';

function usePhotoPreview(file: File | null) {
    const [preview, setPreview] = useState<{ file: File; url: string } | null>(null);
    useEffect(() => {
        if (!file) return;
        const url = URL.createObjectURL(file);
        let active = true;
        queueMicrotask(() => {
            if (active) setPreview({ file, url });
        });
        return () => {
            active = false;
            URL.revokeObjectURL(url);
        };
    }, [file]);
    return preview?.file === file ? preview?.url : undefined;
}

export default function FeedComposer({
    onClose,
    onPublished,
}: {
    onClose: () => void;
    onPublished: (id: string) => void;
}) {
    const id = useId();
    const [file, setFile] = useState<File | null>(null);
    const [caption, setCaption] = useState('');
    const [point, setPoint] = useState<{ lat: number; long: number } | null>(null);
    const preview = usePhotoPreview(file);
    const { publish, pending, error } = useFeedActions('');
    async function submit(event: FormEvent) {
        event.preventDefault();
        if (!file || !point) return;
        const form = new FormData();
        form.append('photo', file);
        form.append('caption', caption.trim());
        form.append('lat', String(point.lat));
        form.append('long', String(point.long));
        const result = await publish(form);
        if (result) onPublished(result.id);
    }
    return (
        <FeedDialog title="Post a geo challenge" onClose={onClose} busy={pending}>
            <p className="feed-note">
                Share a place with everyone on GeoGuessMe. Your photo stays blurred in the feed until each player
                guesses.
            </p>
            <form onSubmit={submit} className="feed-form">
                <fieldset disabled={pending}>
                    <label htmlFor={`${id}-photo`}>
                        Challenge photo
                        <input
                            id={`${id}-photo`}
                            type="file"
                            accept="image/jpeg,image/png,image/webp"
                            required
                            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                        />
                    </label>
                    {preview && <img className="feed-compose-preview" src={preview} alt="Photo to publish" />}
                    <label htmlFor={`${id}-caption`}>
                        Caption
                        <textarea
                            id={`${id}-caption`}
                            maxLength={500}
                            value={caption}
                            placeholder="Give them a clue. Keep the answer a mystery."
                            onChange={(e) => setCaption(e.target.value)}
                        />
                    </label>
                    <h3>Where was this photo taken?</h3>
                    <LocationPicker onChange={setPoint} />
                    <p className="feed-note">
                        The exact location is revealed after a guess. Publish only a place you want to share publicly.
                    </p>
                </fieldset>
                {error && (
                    <p role="alert" className="error-message">
                        {error}
                    </p>
                )}
                <button className="btn btn-primary" type="submit" disabled={pending || !file || !point}>
                    {pending ? 'Publishing…' : 'Publish challenge'}
                </button>
            </form>
        </FeedDialog>
    );
}
