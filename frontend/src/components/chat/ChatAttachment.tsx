import { useEffect, useState } from 'react';
import api from '../../api';
import FullScreenImage from '../ui/FullScreenImage';

interface ChatAttachmentProps {
    mediaID: string;
    mediaType: string;
}

// Chat attachments are private API resources, so native img/video URLs cannot
// carry the bearer header. Fetch a short-lived blob URL and revoke it whenever
// the component changes or unmounts. Images open full screen on click via the
// shared FullScreenImage dialog, like the challenge photo on the results page.
export default function ChatAttachment({ mediaID, mediaType }: ChatAttachmentProps) {
    const [url, setURL] = useState('');
    const [error, setError] = useState('');

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

    if (error) return <p className="chat-attachment-error">{error}</p>;
    if (!url) return <p className="chat-attachment-loading">Loading attachment…</p>;
    if (mediaType.startsWith('video/')) {
        return <video className="chat-attachment" controls preload="metadata" src={url} />;
    }
    return (
        <FullScreenImage src={url} alt="Shared photo">
            <img className="chat-attachment" src={url} alt="Shared photo" />
        </FullScreenImage>
    );
}
