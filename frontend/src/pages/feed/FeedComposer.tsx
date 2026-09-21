import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { groupsAPI } from '../../api';
import Camera from '../../components/camera/Camera';
import type { GroupInbox, MediaProcessingJob } from '../../types';
import { useFeedActions } from './useFeed';
import FeedDialog from './FeedDialog';
import './FeedAudience.css';

/**
 * Public-feed capture deliberately delegates camera, device location, and
 * media cleanup to the same Camera workflow used by group challenges. The
 * feed owns only its audience and caption metadata.
 */
export default function FeedComposer({
    onClose,
    onPublished,
}: {
    onClose: () => void;
    onPublished: (id: string) => void;
}) {
    const id = useId();
    const [caption, setCaption] = useState('');
    const [audience, setAudience] = useState<'public' | 'friends'>('public');
    const [groups, setGroups] = useState<GroupInbox[]>([]);
    const [groupsError, setGroupsError] = useState('');
    const [selectedGroups, setSelectedGroups] = useState<string[]>([]);
    const publishedID = useRef<string | null>(null);
    const { publish, pending, error } = useFeedActions('');

    useEffect(() => {
        const controller = new AbortController();
        let active = true;
        queueMicrotask(() => {
            void groupsAPI
                .inbox(controller.signal)
                .then((items) => {
                    if (!active) return;
                    setGroups(items);
                    setGroupsError('');
                })
                .catch((requestError: unknown) => {
                    if (active && !controller.signal.aborted)
                        setGroupsError(
                            requestError instanceof Error ? requestError.message : 'Unable to load your groups.',
                        );
                });
        });
        return () => {
            active = false;
            controller.abort();
        };
    }, []);

    const uploadFeedChallenge = useCallback(
        async (blob: Blob, filename: string, position: GeolocationPosition): Promise<MediaProcessingJob | null> => {
            const form = new FormData();
            form.append('photo', blob, filename);
            form.append('caption', caption.trim());
            form.append('audience', audience);
            selectedGroups.forEach((groupID) => form.append('group_id', groupID));
            form.append('lat', String(position.coords.latitude));
            form.append('long', String(position.coords.longitude));
            const result = await publish(form);
            if (!result) throw new Error('Unable to publish this challenge. Please try again.');
            publishedID.current = result.id;
            return null;
        },
        [audience, caption, publish, selectedGroups],
    );

    return (
        <FeedDialog title="Post a geo challenge" onClose={onClose} busy={pending}>
            <p className="feed-note">
                Take a photo of a place now. Your device location is attached automatically and the photo stays blurred
                until each player guesses.
            </p>
            <div className="feed-form feed-camera-form">
                <fieldset disabled={pending} className="feed-capture-details">
                    <label htmlFor={`${id}-caption`}>
                        Caption
                        <textarea
                            id={`${id}-caption`}
                            maxLength={280}
                            value={caption}
                            placeholder="Give them a clue. Keep the answer a mystery."
                            onChange={(event) => setCaption(event.target.value)}
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
                        {audience === 'friends' && (
                            <div className="feed-group-targets">
                                <span>Limit to selected groups (optional)</span>
                                {groupsError && <p className="error-message">{groupsError}</p>}
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
                </fieldset>
                <Camera
                    variant="feed"
                    uploadCaptured={uploadFeedChallenge}
                    onUploadComplete={() => {
                        const postID = publishedID.current;
                        if (postID) onPublished(postID);
                    }}
                />
                <p className="feed-note">
                    The exact device location is used for this challenge. Feed posts do not support choosing an old
                    photo or entering a location manually.
                </p>
                {error && (
                    <p role="alert" className="error-message">
                        {error}
                    </p>
                )}
            </div>
        </FeedDialog>
    );
}
