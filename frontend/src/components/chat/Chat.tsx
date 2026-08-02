import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import api, { getAPIErrorMessage } from '../../api';
import type { Message } from '../../types';
import Avatar from '../common/Avatar';
import Icon from '../ui/Icon';
import ChatAttachment from './ChatAttachment';
import ChallengeTimer from './ChallengeTimer';
import { reactionByKey, reactionOptions } from './reactionOptions';
import './Chat.css';
import './ChatActions.css';

interface ChatProps {
    messages: Message[];
    wsRef: React.RefObject<WebSocket | null>;
    currentUserId: string;
    groupID: string;
    connectionStatus?: 'connecting' | 'connected' | 'offline';
    onChallengeMessage?: (message: Message) => void;
    onMessageUpdated?: (message: Message) => void;
}

// How long a touch must be held before the reply/react panel opens. This is
// the deliberate long press that replaces tap-to-open on touch devices.
const LONG_PRESS_MS = 1000;

interface PressState {
    messageID: string;
    startX: number;
    startY: number;
    timer: number;
}

export default function Chat({
    messages,
    wsRef,
    currentUserId,
    groupID,
    connectionStatus = 'offline',
    onChallengeMessage,
    onMessageUpdated,
}: ChatProps) {
    const [input, setInput] = useState('');
    const [replyingTo, setReplyingTo] = useState<Message | null>(null);
    const [attachment, setAttachment] = useState<File | null>(null);
    const [uploadError, setUploadError] = useState('');
    const [uploading, setUploading] = useState(false);
    const [actionsMessageID, setActionsMessageID] = useState<string | null>(null);
    const [reactionPending, setReactionPending] = useState<string | null>(null);
    const [reactionError, setReactionError] = useState('');
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const pressRef = useRef<PressState | null>(null);
    // Hover-capable devices reveal actions on hover and keyboard focus; touch
    // devices rely on a deliberate long press instead, like Messenger or
    // WhatsApp, so a plain tap never opens the reply/react panel.
    const [canHover] = useState(
        () => typeof window !== 'undefined' && !!window.matchMedia && window.matchMedia('(hover: hover)').matches,
    );

    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages]);

    useEffect(() => {
        const dismissActions = (event: PointerEvent) => {
            if (!(event.target as HTMLElement).closest('[data-message-id]')) {
                setActionsMessageID(null);
            }
        };
        document.addEventListener('pointerdown', dismissActions);
        return () => document.removeEventListener('pointerdown', dismissActions);
    }, []);

    const hoverHandlers = (messageID: string, enabled: boolean) =>
        canHover && enabled
            ? {
                  onMouseEnter: () => setActionsMessageID(messageID),
                  onMouseLeave: () => setActionsMessageID(null),
              }
            : {};

    const clearPress = () => {
        if (pressRef.current) window.clearTimeout(pressRef.current.timer);
        pressRef.current = null;
    };

    const handleMessagePointerDown = (event: React.PointerEvent<HTMLDivElement>, messageID: string) => {
        if (!event.isPrimary || (event.target as HTMLElement).closest('button')) return;
        setActionsMessageID((current) => (current === messageID ? current : null));
        clearPress();
        const timer = window.setTimeout(() => {
            setActionsMessageID(messageID);
        }, LONG_PRESS_MS);
        pressRef.current = { messageID, startX: event.clientX, startY: event.clientY, timer };
    };

    const handleMessagePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
        const press = pressRef.current;
        if (!press || press.messageID !== event.currentTarget.dataset.messageId) return;
        const horizontal = Math.abs(event.clientX - press.startX);
        const vertical = Math.abs(event.clientY - press.startY);
        if (vertical > 20 && vertical > horizontal) {
            clearPress();
            return;
        }
        if (horizontal > 28 && horizontal > vertical) {
            setActionsMessageID(press.messageID);
            clearPress();
        }
    };

    const handleMessagePointerEnd = () => clearPress();

    const handleReaction = async (message: Message, reaction: string) => {
        const selected = message.reactions?.find((item) => item.reaction === reaction);
        const key = `${message.id}:${reaction}`;
        setReactionPending(key);
        setReactionError('');
        try {
            const response = selected?.reacted
                ? await api.delete<Message>(`/group/message-reactions/${message.id}`, { data: { reaction } })
                : await api.put<Message>(`/group/message-reactions/${message.id}`, { reaction });
            onMessageUpdated?.(response.data);
            setActionsMessageID(null);
        } catch (requestError: unknown) {
            setReactionError(getAPIErrorMessage(requestError, 'Unable to save reaction'));
        } finally {
            setReactionPending(null);
        }
    };

    const renderMessageActions = (message: Message) => (
        <div className="message-actions" aria-label="Message actions">
            <button
                type="button"
                className="reply-action"
                tabIndex={actionsMessageID === message.id ? 0 : -1}
                onClick={() => {
                    setReplyingTo(message);
                    setActionsMessageID(null);
                }}
                aria-label={`Reply to ${message.username || 'message'}`}
            >
                Reply
            </button>
            <div className="reaction-actions" aria-label="React to message">
                {reactionOptions.map(({ reaction, label, image }) => {
                    const item = message.reactions?.find((entry) => entry.reaction === reaction);
                    const pending = reactionPending === `${message.id}:${reaction}`;
                    return (
                        <button
                            key={reaction}
                            type="button"
                            className={`reaction-action${item?.reacted ? ' selected' : ''}`}
                            tabIndex={actionsMessageID === message.id ? 0 : -1}
                            onClick={() => void handleReaction(message, reaction)}
                            aria-label={`React with ${label}`}
                            aria-pressed={item?.reacted ?? false}
                            disabled={pending}
                            title={label}
                        >
                            <img src={image} alt="" className="reaction-action-image" />
                        </button>
                    );
                })}
            </div>
        </div>
    );

    const renderReactions = (message: Message) =>
        message.reactions && message.reactions.length > 0 ? (
            <div className="message-reactions" aria-label="Message reactions">
                {message.reactions.map((reaction) => {
                    const option = reactionByKey.get(reaction.reaction);
                    const label = option?.label ?? reaction.reaction;
                    const usernames = reaction.usernames?.length ? reaction.usernames.join(', ') : 'Unknown user';
                    const reactionLabel = `${label} reaction, ${reaction.count}. Reacted by ${usernames}`;
                    return (
                        <span key={reaction.reaction} className="reaction-chip-wrapper">
                            <button
                                type="button"
                                className={`reaction-chip${reaction.reacted ? ' selected' : ''}`}
                                onClick={() => void handleReaction(message, reaction.reaction)}
                                aria-label={reactionLabel}
                                title={`Reacted by ${usernames}`}
                                aria-pressed={reaction.reacted}
                            >
                                {option ? (
                                    <img src={option.image} alt="" className="reaction-chip-image" />
                                ) : (
                                    <span aria-hidden="true">{reaction.reaction}</span>
                                )}
                                <span>{reaction.count}</span>
                            </button>
                            <span className="reaction-chip-tooltip" role="tooltip">
                                {usernames}
                            </span>
                        </span>
                    );
                })}
            </div>
        ) : null;

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
                    const previousMessage = index > 0 ? messages[index - 1] : undefined;
                    const sameSenderGroup =
                        !isSystem && previousMessage?.kind !== 'system' && previousMessage?.user_id === message.user_id;
                    const showSender = !sameSenderGroup;
                    const replyTarget = message.reply_to_id
                        ? messages.find((candidate) => candidate.id === message.reply_to_id)
                        : undefined;
                    if (message.kind === 'challenge') {
                        return (
                            <div
                                key={message.id}
                                data-message-id={message.id}
                                className={`message-container ${isMe ? 'own' : 'other'} ${actionsMessageID === message.id ? 'actions-visible' : ''} slide-in-up`}
                                tabIndex={0}
                                onFocus={() => setActionsMessageID(message.id)}
                                onPointerDown={(event) => handleMessagePointerDown(event, message.id)}
                                onPointerMove={handleMessagePointerMove}
                                onPointerUp={handleMessagePointerEnd}
                                onPointerCancel={handleMessagePointerEnd}
                            >
                                {!isMe && (
                                    <div
                                        className={`avatar-container${showSender ? '' : ' avatar-placeholder'}`}
                                        aria-hidden={!showSender}
                                    >
                                        {showSender && (
                                            <Link
                                                to={`/profile/${message.user_id}`}
                                                aria-label={`View ${message.username || 'player'}'s profile`}
                                            >
                                                <Avatar
                                                    userID={message.user_id}
                                                    avatar={message.avatar}
                                                    username={message.username}
                                                    className="avatar"
                                                />
                                            </Link>
                                        )}
                                    </div>
                                )}
                                <div className="message-wrapper">
                                    {!isMe && showSender && (
                                        <Link className="message-username" to={`/profile/${message.user_id}`}>
                                            {message.username || 'Unknown User'}
                                        </Link>
                                    )}
                                    <div className="message-hover-target" {...hoverHandlers(message.id, true)}>
                                        <button
                                            className={`message-content photo-challenge clickable${message.challenge_status === 'guessed' ? ' resolved' : ''}`}
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
                                                    <span>
                                                        {message.challenge_status === 'guessed'
                                                            ? 'Resolved challenge'
                                                            : message.challenge_status === 'expired'
                                                              ? 'Challenge expired'
                                                              : isMe
                                                                ? 'Challenge sent'
                                                                : 'New challenge'}
                                                    </span>
                                                    {message.challenge_expires_at && message.challenge_ttl_seconds ? (
                                                        <ChallengeTimer
                                                            expiresAt={message.challenge_expires_at}
                                                            ttlSeconds={message.challenge_ttl_seconds}
                                                        />
                                                    ) : null}
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
                                        {renderMessageActions(message)}
                                    </div>
                                    {renderReactions(message)}
                                </div>
                            </div>
                        );
                    }
                    return (
                        <div
                            key={message.id}
                            data-message-id={message.id}
                            className={`message-container ${isMe ? 'own' : 'other'} ${isSystem ? 'system' : ''} ${actionsMessageID === message.id ? 'actions-visible' : ''} slide-in-up`}
                            tabIndex={isSystem ? -1 : 0}
                            onFocus={() => !isSystem && setActionsMessageID(message.id)}
                            onPointerDown={(event) => handleMessagePointerDown(event, message.id)}
                            onPointerMove={handleMessagePointerMove}
                            onPointerUp={handleMessagePointerEnd}
                            onPointerCancel={handleMessagePointerEnd}
                        >
                            {!isMe && !isSystem && (
                                <div
                                    className={`avatar-container${showSender ? '' : ' avatar-placeholder'}`}
                                    aria-hidden={!showSender}
                                >
                                    {showSender && (
                                        <Link
                                            to={`/profile/${message.user_id}`}
                                            aria-label={`View ${message.username || 'player'}'s profile`}
                                        >
                                            <Avatar
                                                userID={message.user_id}
                                                avatar={message.avatar}
                                                username={message.username}
                                                className="avatar"
                                            />
                                        </Link>
                                    )}
                                </div>
                            )}
                            <div className="message-wrapper">
                                {!isMe && !isSystem && showSender && (
                                    <Link className="message-username" to={`/profile/${message.user_id}`}>
                                        {message.username || 'Unknown User'}
                                    </Link>
                                )}
                                <div className="message-hover-target" {...hoverHandlers(message.id, !isSystem)}>
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
                                    {!isSystem && renderMessageActions(message)}
                                </div>
                                {!isSystem && renderReactions(message)}
                            </div>
                        </div>
                    );
                })}
                <div ref={messagesEndRef} />
            </div>
            {reactionError && (
                <p className="chat-reaction-error" role="alert">
                    {reactionError}
                </p>
            )}
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
