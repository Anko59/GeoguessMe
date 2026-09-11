import { useEffect, useRef, useState } from 'react';
import { publicFeedAPI } from '../../api';

function useFeedMedia(id: string, playing: boolean, revealed: boolean) {
    const [asset, setAsset] = useState<{ key: string; url: string } | null>(null);
    const [failedKey, setFailedKey] = useState('');
    const [attempt, setAttempt] = useState(0);
    const key = `${id}:${playing}:${revealed}:${attempt}`;
    useEffect(() => {
        const controller = new AbortController();
        let url: string | undefined;
        void publicFeedAPI
            .media(id, playing, controller.signal)
            .then((blob) => {
                if (controller.signal.aborted) return;
                url = URL.createObjectURL(blob);
                setAsset({ key, url });
            })
            .catch(() => {
                if (!controller.signal.aborted) setFailedKey(key);
            });
        return () => {
            controller.abort();
            if (url) URL.revokeObjectURL(url);
        };
    }, [id, playing, key]);
    return {
        url: asset?.key === key ? asset.url : null,
        failed: failedKey === key,
        retry: () => setAttempt((a) => a + 1),
    };
}

function LoadedFeedImage({ id, revealed, playing = false }: { id: string; revealed: boolean; playing?: boolean }) {
    const { url, failed, retry } = useFeedMedia(id, playing, revealed);
    if (failed)
        return (
            <div className="feed-image-state" role="alert">
                Photo unavailable.{' '}
                <button className="btn btn-secondary" onClick={retry}>
                    Retry photo
                </button>
            </div>
        );
    if (!url)
        return (
            <div className="feed-image-state" role="status">
                Loading photo…
            </div>
        );
    return (
        <img
            src={url}
            alt={revealed || playing ? 'Geo challenge photo' : 'Blurred preview of an unsolved geo challenge'}
            className={revealed || playing ? 'feed-photo' : 'feed-photo feed-photo-blurred'}
        />
    );
}

// Mount media only near the viewport. Scrolling away releases decoded images
// and blob URLs, so loading more feed pages cannot retain every full photo.
export default function FeedImage(props: { id: string; revealed: boolean; playing?: boolean }) {
    const frame = useRef<HTMLDivElement>(null);
    const [visible, setVisible] = useState(typeof IntersectionObserver === 'undefined');
    useEffect(() => {
        if (typeof IntersectionObserver === 'undefined' || !frame.current) return;
        let active = true;
        const observer = new IntersectionObserver(
            (entries) => {
                if (active) setVisible(entries.some((entry) => entry.isIntersecting));
            },
            { rootMargin: '300px' },
        );
        observer.observe(frame.current);
        return () => {
            active = false;
            observer.disconnect();
        };
    }, []);
    return (
        <div className="feed-photo-frame" ref={frame}>
            {visible ? <LoadedFeedImage {...props} /> : <div className="feed-image-state" aria-hidden="true" />}
        </div>
    );
}
