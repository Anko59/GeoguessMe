import { useEffect, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import type { Message } from '../../types';
import Icon from '../ui/Icon';
import ChatAttachment from './ChatAttachment';
import './Chat.css';

interface ChatProps {
    messages: Message[];
    wsRef: React.RefObject<WebSocket | null>;
    currentUserId: string;
    groupID: string;
    connectionStatus?: 'connecting' | 'connected' | 'offline';
    onChallengeMessage?: (message: Message) => void;
}

export default function Chat({
    messages,
    wsRef,
    currentUserId,
    groupID,
    connectionStatus = 'offline',
    onChallengeMessage,
}: ChatProps) {
    const [input, setInput] = useState('');
    const [replyingTo, setReplyingTo] = useState<Message | null>(null);
    const [attachment, setAttachment] = useState<File | null>(null);
    const [uploadError, setUploadError] = useState('');
    const [uploading, setUploading] = useState(false);
    const messagesEndRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages]);

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
                    setReplyingTo(null);
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
        setReplyingTo(null);
    };

    return (
        <div className="chat-container">
            <div className={`chat-status ${connectionStatus}`} role="status" aria-live="polite">
                {connectionStatus === 'connected'
                    ? 'Connected'
                    : connectionStatus === 'connecting'
                      ? 'Connecting…'
                      : 'Offline — retrying'}
            </div>
            <div className="messages-list">
                {messages.length === 0 && (
                    <div className="chat-empty-state">
                        <img src="/chat_bubbl_icon.png" alt="" className="chat-empty-icon" />
                        <h2>No messages yet</h2>
                        <p className="empty-subtitle">Start chatting with your group!</p>
                    </div>
                )}
                {messages.map((message, index) => {
                    const isMe = message.user_id === currentUserId;
                    const isSystem = message.kind === 'system';
                    const showAvatar = index === 0 || messages[index - 1].user_id !== message.user_id;
                    const replyTarget = message.reply_to_id
                        ? messages.find((candidate) => candidate.id === message.reply_to_id)
                        : undefined;
                    if (message.kind === 'challenge') {
                        return (
                            <div
                                key={message.id}
                                data-message-id={message.id}
                                className={`message-container ${isMe ? 'own' : 'other'} slide-in-up`}
                            >
                                {!isMe && showAvatar && (
                                    <div className="avatar-container">
                                        <img src={`/avatars/${message.avatar || 'avatar.png'}`} alt="" />
                                    </div>
                                )}
                                <div className="message-wrapper">
                                    {!isMe && (
                                        <div className="message-username">{message.username || 'Unknown User'}</div>
                                    )}
                                    <button
                                        className="message-content photo-challenge clickable"
                                        data-photo-id={message.photo_id}
                                        onClick={() => onChallengeMessage?.(message)}
                                    >
                                        <span className="challenge-card">
                                            <span className="challenge-header">
                                                <img
                                                    src={
                                                        isMe
                                                            ? '/challenge_sent_icon.png'
                                                            : '/challenge_received_icon.png'
                                                    }
                                                    alt=""
                                                    className="challenge-icon"
                                                />
                                                <span>{isMe ? 'Challenge sent' : 'New challenge'}</span>
                                            </span>
                                            <span className="start-challenge-btn">
                                                {isMe ||
                                                message.challenge_status === 'results' ||
                                                message.challenge_status === 'guessed' ||
                                                message.challenge_status === 'expired'
                                                    ? 'View results'
                                                    : message.challenge_status === 'accepted'
                                                      ? 'Continue challenge'
                                                      : 'Accept challenge'}
                                            </span>
                                        </span>
                                    </button>
                                    <button
                                        type="button"
                                        className="reply-action"
                                        onClick={() => setReplyingTo(message)}
                                        aria-label={`Reply to ${message.username || 'message'}`}
                                    >
                                        Reply
                                    </button>
                                </div>
                            </div>
                        );
                    }
                    return (
                        <div
                            key={message.id}
                            data-message-id={message.id}
                            className={`message-container ${isMe ? 'own' : 'other'} ${isSystem ? 'system' : ''} slide-in-up`}
                        >
                            {!isMe && !isSystem && showAvatar && (
                                <div className="avatar-container">
                                    <img src={`/avatars/${message.avatar || 'avatar.png'}`} alt="" className="avatar" />
                                </div>
                            )}
                            <div className="message-wrapper">
                                {!isMe && !isSystem && (
                                    <div className="message-username">{message.username || 'Unknown User'}</div>
                                )}
                                <div
                                    className={`message-content ${isSystem ? 'system-message' : message.kind === 'media' ? 'media-message' : 'text'}`}
                                >
                                    {message.reply_to_id && (
                                        <div className="reply-context">
                                            <strong>{replyTarget?.username || 'Original message'}</strong>
                                            <span>{replyTarget?.content || 'Message unavailable'}</span>
                                        </div>
                                    )}
                                    {message.kind === 'media' && message.media_id && message.media_type && (
                                        <ChatAttachment mediaID={message.media_id} mediaType={message.media_type} />
                                    )}
                                    {message.content && <p className="message-caption">{message.content}</p>}
                                </div>
                                {!isSystem && (
                                    <button
                                        type="button"
                                        className="reply-action"
                                        onClick={() => setReplyingTo(message)}
                                        aria-label={`Reply to ${message.username || 'message'}`}
                                    >
                                        Reply
                                    </button>
                                )}
                            </div>
                        </div>
                    );
                })}
                <div ref={messagesEndRef} />
            </div>
            <form onSubmit={sendMessage} className="message-input-container">
                {replyingTo && (
                    <div className="reply-composer" role="status">
                        <span>
                            Replying to <strong>{replyingTo.username || 'message'}</strong>
                        </span>
                        <button type="button" onClick={() => setReplyingTo(null)} aria-label="Cancel reply">
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
        </div>
    );
}
