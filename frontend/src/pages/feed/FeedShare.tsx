import { useEffect, useId, useRef, useState } from 'react';

export default function FeedShare({ id, username }: { id: string; username: string }) {
    const inputID = useId();
    const [status, setStatus] = useState<'idle' | 'pending' | 'copied' | 'manual'>('idle');
    const active = useRef(true);
    useEffect(() => {
        active.current = true;
        return () => {
            active.current = false;
        };
    }, []);
    const url = `${window.location.origin}/feed/${encodeURIComponent(id)}`;
    async function share() {
        if (status === 'pending') return;
        setStatus('pending');
        try {
            if (navigator.share) {
                await navigator.share({
                    title: 'GeoGuessMe challenge',
                    text: `Can you find this place? A challenge by ${username}.`,
                    url,
                });
                if (active.current) setStatus('idle');
            } else if (navigator.clipboard?.writeText) {
                await navigator.clipboard.writeText(url);
                if (active.current) setStatus('copied');
            } else if (active.current) setStatus('manual');
        } catch (error) {
            if (active.current) setStatus(error instanceof Error && error.name === 'AbortError' ? 'idle' : 'manual');
        }
    }
    return (
        <div className="feed-share">
            <button
                className="feed-icon-button"
                aria-label="Share challenge"
                disabled={status === 'pending'}
                onClick={() => void share()}
            >
                <img src="/foward_arrow_icon.png" alt="" aria-hidden="true" className="feed-action-icon" />
            </button>
            {status === 'copied' && <span role="status">Link copied</span>}
            {status === 'manual' && (
                <label htmlFor={inputID}>
                    Copy this link to share
                    <input id={inputID} readOnly value={url} onFocus={(event) => event.currentTarget.select()} />
                </label>
            )}
        </div>
    );
}
