import { useCallback, useRef } from 'react';
import Camera from '../../components/camera/Camera';
import type { CaptureUploadOptions } from '../../components/camera/useChallengeUpload';
import type { MediaProcessingJob } from '../../types';
import { useFeedActions } from './useFeed';
import FeedDialog from './FeedDialog';
import './FeedAudience.css';

/**
 * Feed capture deliberately owns no alternate capture UI. Camera is the
 * same capture, preview, retake, and options surface used for group
 * challenges; the callback only adapts its destinations to the feed API.
 */
export default function FeedComposer({
    onClose,
    onPublished,
}: {
    onClose: () => void;
    onPublished: (id: string) => void;
}) {
    const publishedID = useRef<string | null>(null);
    const { publish, pending, error } = useFeedActions('');

    const uploadFeedChallenge = useCallback(
        async (
            blob: Blob,
            filename: string,
            position: GeolocationPosition,
            options: CaptureUploadOptions,
        ): Promise<MediaProcessingJob | null> => {
            const form = new FormData();
            form.append('photo', blob, filename);
            form.append('caption', options.caption.trim());
            form.append('audience', options.audience);
            options.groupIDs.forEach((groupID) => form.append('group_id', groupID));
            form.append('hide_location', String(options.hideLocation));
            form.append('idempotency_key', options.idempotencyKey);
            form.append('lat', String(position.coords.latitude));
            form.append('long', String(position.coords.longitude));
            const result = await publish(form);
            if (!result) throw new Error('Unable to publish this challenge. Please try again.');
            publishedID.current = result.id;
            return null;
        },
        [publish],
    );

    return (
        <FeedDialog title="Post a geo challenge" className="feed-camera-dialog" onClose={onClose} busy={pending}>
            <div className="feed-form feed-camera-form">
                <Camera
                    variant="feed"
                    uploadCaptured={uploadFeedChallenge}
                    onUploadComplete={() => {
                        const postID = publishedID.current;
                        if (postID) onPublished(postID);
                    }}
                />
                <p className="feed-note feed-camera-note">
                    The exact device location is attached automatically. Feed posts are camera-only and do not support
                    choosing an old photo or entering a location manually.
                </p>
                {error && (
                    <p role="alert" className="error-message feed-camera-error">
                        {error}
                    </p>
                )}
            </div>
        </FeedDialog>
    );
}
