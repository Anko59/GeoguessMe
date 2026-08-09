import { useState } from 'react';
import api, { getAPIErrorMessage } from '../../../api';
import type { Message } from '../../../types';
import Icon from '../../ui/Icon';
import type { ConnectionStatus } from '../../../hooks/useGroupMessages';

interface ComposerProps {
    wsRef: React.RefObject<WebSocket | null>;
    groupID: string;
    connectionStatus: ConnectionStatus;
    /** The message being replied to, shown above the input. */
    replyingTo: Message | null;
    /** Clear the reply context (cancel, or after a successful send). */
    onCancelReply: () => void;
}

/** The chat composer: reply context, attachment picker, text input, and send.
 *  Owns its own draft state; the server stays authoritative for upload size. */
export default function Composer({ wsRef, groupID, connectionStatus, replyingTo, onCancelReply }: ComposerProps) {
    const [input, setInput] = useState('');
    const [attachment, setAttachment] = useState<File | null>(null);
    const [uploadError, setUploadError] = useState('');
    const [uploading, setUploading] = useState(false);

    const sendMessage = (event: React.FormEvent): void => {
        event.preventDefault();
        const content = input.trim();
        if (wsRef.current?.readyState !== WebSocket.OPEN) return;
        if (attachment) {
            const body = new FormData();
            body.set('group_id', groupID);
            body.set('media', attachment);
            if (content) body.set('content', content);
            if (replyingTo) body.set('reply_to_id', replyingTo.id);
            setUploading(true);
            setUploadError('');
            void api
                .post('/group/messages/media', body)
                .then(() => {
                    setAttachment(null);
                    setInput('');
                    onCancelReply();
                })
                .catch((requestError: unknown) =>
                    setUploadError(getAPIErrorMessage(requestError, 'Unable to send attachment')),
                )
                .finally(() => setUploading(false));
            return;
        }
        if (!content) return;
        wsRef.current.send(JSON.stringify({ content, ...(replyingTo ? { reply_to_id: replyingTo.id } : {}) }));
        setInput('');
        onCancelReply();
    };

    return (
        <form onSubmit={sendMessage} className="message-input-container">
            {replyingTo && (
                <div className="reply-composer" role="status">
                    <span>
                        Replying to <strong>{replyingTo.username || 'message'}</strong>
                    </span>
                    <button type="button" onClick={onCancelReply} aria-label="Cancel reply">
                        Cancel
                    </button>
                </div>
            )}
            {uploadError && (
                <p className="chat-upload-error" role="alert">
                    {uploadError}
                </p>
            )}
            {attachment && (
                <div className="attachment-composer" role="status">
                    <span>{attachment.name}</span>
                    <button type="button" onClick={() => setAttachment(null)} disabled={uploading}>
                        Remove
                    </button>
                </div>
            )}
            <label className="attachment-button" htmlFor="chat-attachment">
                <span className="visually-hidden">Attach photo or video</span>
                <input
                    id="chat-attachment"
                    aria-label="Attach photo or video"
                    type="file"
                    accept="image/jpeg,image/png,video/mp4,video/webm"
                    onChange={(event) => setAttachment(event.target.files?.[0] ?? null)}
                    disabled={connectionStatus !== 'connected' || uploading}
                />
                <span aria-hidden="true">+</span>
            </label>
            <label htmlFor="chat-message" className="visually-hidden">
                Message
            </label>
            <input
                id="chat-message"
                type="text"
                value={input}
                onChange={(event) => setInput(event.target.value)}
                placeholder="Type a message…"
                className="message-input"
                maxLength={1000}
                disabled={connectionStatus !== 'connected' || uploading}
            />
            <button
                type="submit"
                className="send-button"
                disabled={(!input.trim() && !attachment) || connectionStatus !== 'connected' || uploading}
                aria-label={attachment ? 'Send attachment' : 'Send message'}
            >
                <Icon name="send" className="send-icon" />
            </button>
        </form>
    );
}
