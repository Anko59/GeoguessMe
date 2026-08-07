import { useEffect, useState } from 'react';
import api from '../../api';
import Icon from '../ui/Icon';
import './ChatAttachment.css';

interface ChatAttachmentProps {
    mediaID: string;
    mediaType: string;
}

// Chat attachments are private API resources, so native img/video URLs cannot
// carry the bearer header. Fetch a short-lived blob URL and revoke it whenever
// the component changes or unmounts. Images open full screen on click, like
// the challenge photo on the results page.
export default function ChatAttachment({ mediaID, mediaType }: ChatAttachmentProps) {
    const [url, setURL] = useState('');
    const [error, setError] = useState('');
    const [expanded, setExpanded] = useState(false);

    useEffect(() => {
        let active = true;
        let objectURL = '';
        void api
            .get(`/group/messages/media/${encodeURIComponent(mediaID)}`, { responseType: 'blob' })
            .then((response) => {
                objectURL = URL.createObjectURL(response.data as Blob);
                if (active) setURL(objectURL);
            })
            .catch(() => {
                if (active) setError('Unable to load attachment');
            });
        return () => {
            active = false;
            if (objectURL) URL.revokeObjectURL(objectURL);
        };
    }, [mediaID]);

    useEffect(() => {
        if (!expanded) return undefined;
        const closeOnEscape = (event: KeyboardEvent) => {
            if (event.key === 'Escape') setExpanded(false);
        };
        window.addEventListener('keydown', closeOnEscape);
        return () => window.removeEventListener('keydown', closeOnEscape);
    }, [expanded]);

    if (error) return <p className="chat-attachment-error">{error}</p>;
    if (!url) return <p className="chat-attachment-loading">Loading attachment…</p>;
    if (mediaType.startsWith('video/')) {
        return <video className="chat-attachment" controls preload="metadata" src={url} />;
    }
    return (
        <>
            <button
                type="button"
                className="chat-attachment-expand"
                onClick={() => setExpanded(true)}
                aria-label="View photo full screen"
            >
                <img className="chat-attachment" src={url} alt="Shared photo" />
            </button>
            {expanded && (
                <div
                    className="chat-image-dialog"
                    role="dialog"
                    aria-modal="true"
                    aria-label="Shared photo full screen"
                >
                    <button
                        type="button"
                        className="chat-image-dialog-close"
                        onClick={() => setExpanded(false)}
                        aria-label="Close full-screen photo"
                    >
                        <Icon name="close" />
                    </button>
                    <img src={url} alt="Shared photo full screen" className="chat-image-dialog-photo" />
                </div>
            )}
        </>
    );
}
