import { useEffect, useId, useRef, useState, type FormEvent, type RefObject } from 'react';
import { groupsAPI } from '../../api';
import type { GroupInbox } from '../../types';
import { useFeedActions } from './useFeed';
import FeedDialog from './FeedDialog';
import LocationPicker from './LocationPicker';
import './FeedAudience.css';

function usePhotoPreview(file: File | null, canvas: RefObject<HTMLCanvasElement | null>) {
    const [preview, setPreview] = useState<{ file: File; error: string | null } | null>(null);
    useEffect(() => {
        if (!file) return;
        let active = true;
        async function drawPreview(photo: File) {
            if (!['image/jpeg', 'image/png', 'image/webp'].includes(photo.type)) {
                throw new Error('Unsupported photo type');
            }
            // Display decoded pixels only. Never expose a URL for the raw file,
            // whose content type and contents are controlled by the uploader.
            const bitmap = await createImageBitmap(photo);
            try {
                const target = canvas.current;
                if (!active || !target) return;
                const context = target.getContext('2d');
                if (!context) throw new Error('Photo preview unavailable');
                const scale = Math.min(1, 640 / bitmap.width, 240 / bitmap.height);
                target.width = Math.max(1, Math.round(bitmap.width * scale));
                target.height = Math.max(1, Math.round(bitmap.height * scale));
                context.drawImage(bitmap, 0, 0, target.width, target.height);
                setPreview({ file: photo, error: null });
            } finally {
                bitmap.close();
            }
        }
        void drawPreview(file).catch(() => {
            if (active) setPreview({ file, error: 'Choose a valid JPG, PNG, or WebP photo.' });
        });
        return () => {
            active = false;
        };
    }, [file, canvas]);
    const current = preview?.file === file ? preview : null;
    return { ready: current?.error === null, error: current?.error };
}

export default function FeedComposer({
    onClose,
    onPublished,
}: {
    onClose: () => void;
    onPublished: (id: string) => void;
}) {
    const id = useId();
    const canvas = useRef<HTMLCanvasElement>(null);
    const [file, setFile] = useState<File | null>(null);
    const [caption, setCaption] = useState('');
    const [audience, setAudience] = useState<'public' | 'friends'>('public');
    const [groups, setGroups] = useState<GroupInbox[]>([]);
    const [selectedGroups, setSelectedGroups] = useState<string[]>([]);
    const [point, setPoint] = useState<{ lat: number; long: number } | null>(null);
    const preview = usePhotoPreview(file, canvas);
    const { publish, pending, error } = useFeedActions('');
    useEffect(() => {
        const controller = new AbortController();
        let active = true;
        queueMicrotask(() => {
            void groupsAPI
                .inbox(controller.signal)
                .then((items) => {
                    if (active) setGroups(items);
                })
                .catch(() => undefined);
        });
        return () => {
            active = false;
            controller.abort();
        };
    }, []);
    async function submit(event: FormEvent) {
        event.preventDefault();
        if (!file || !point || !preview.ready) return;
        const form = new FormData();
        form.append('photo', file);
        form.append('caption', caption.trim());
        form.append('audience', audience);
        selectedGroups.forEach((groupID) => form.append('group_id', groupID));
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
                            aria-invalid={Boolean(preview.error)}
                            aria-describedby={preview.error ? `${id}-photo-error` : undefined}
                            onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                        />
                    </label>
                    <canvas
                        ref={canvas}
                        className="feed-compose-preview"
                        role="img"
                        aria-label="Photo to publish"
                        hidden={!preview.ready}
                    />
                    {file && !preview.ready && !preview.error && <p role="status">Preparing photo…</p>}
                    {preview.error && (
                        <p id={`${id}-photo-error`} role="alert" className="error-message">
                            {preview.error}
                        </p>
                    )}
                    <label htmlFor={`${id}-caption`}>
                        Caption
                        <textarea
                            id={`${id}-caption`}
                            maxLength={280}
                            value={caption}
                            placeholder="Give them a clue. Keep the answer a mystery."
                            onChange={(e) => setCaption(e.target.value)}
                        />
                    </label>
                    <fieldset className="feed-audience" aria-labelledby={`${id}-audience-label`}>
                        <legend id={`${id}-audience-label`}>Who can see this challenge?</legend>
                        <label>
                            <input
                                type="radio"
                                name={`${id}-audience`}
                                value="public"
                                checked={audience === 'public'}
                                onChange={() => {
                                    setAudience('public');
                                    setSelectedGroups([]);
                                }}
                            />
                            Everyone on GeoGuessMe
                        </label>
                        <label>
                            <input
                                type="radio"
                                name={`${id}-audience`}
                                value="friends"
                                checked={audience === 'friends'}
                                onChange={() => setAudience('friends')}
                            />
                            Friends in my groups
                        </label>
                        {audience === 'friends' && groups.length > 0 && (
                            <div className="feed-group-targets">
                                <span>Limit to selected groups (optional)</span>
                                {groups.map((group) => (
                                    <label key={group.id}>
                                        <input
                                            type="checkbox"
                                            checked={selectedGroups.includes(group.id)}
                                            onChange={() =>
                                                setSelectedGroups((current) =>
                                                    current.includes(group.id)
                                                        ? current.filter((selected) => selected !== group.id)
                                                        : [...current, group.id],
                                                )
                                            }
                                        />
                                        {group.name}
                                    </label>
                                ))}
                            </div>
                        )}
                    </fieldset>
                    <h3>Where was this photo taken?</h3>
                    <LocationPicker onChange={setPoint} />
                    <p className="feed-note">
                        The exact location is revealed after a guess. Friends posts are visible to members of shared
                        groups; selecting groups narrows that audience.
                    </p>
                </fieldset>
                {error && (
                    <p role="alert" className="error-message">
                        {error}
                    </p>
                )}
                <button className="btn btn-primary" type="submit" disabled={pending || !preview.ready || !point}>
                    {pending ? 'Publishing…' : 'Publish challenge'}
                </button>
            </form>
        </FeedDialog>
    );
}
