import { useEffect, useRef, useState } from 'react';
import type { Message } from '../../types';
import './Chat.css';

interface ChatProps {
    messages: Message[];
    wsRef: React.RefObject<WebSocket | null>;
    currentUserId: string;
    connectionStatus?: 'connecting' | 'connected' | 'offline';
    onChallengeMessage?: (message: Message) => void;
}

export default function Chat({
    messages,
    wsRef,
    currentUserId,
    connectionStatus = 'offline',
    onChallengeMessage,
}: ChatProps) {
    const [input, setInput] = useState('');
    const messagesEndRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages]);

    const sendMessage = (event: React.FormEvent): void => {
        event.preventDefault();
        const content = input.trim();
        if (!content || wsRef.current?.readyState !== WebSocket.OPEN) return;
        wsRef.current.send(JSON.stringify({ content }));
        setInput('');
    };

    return (
        <div className="chat-container">
            <div className="chat-status visually-hidden" role="status" aria-live="polite">
                {connectionStatus === 'connected'
                    ? 'Connected'
                    : connectionStatus === 'connecting'
                      ? 'Connecting…'
                      : 'Offline — retrying'}
            </div>
            <div className="messages-list">
                {messages.length === 0 && (
                    <div className="empty-state">
                        <div className="empty-icon">💬</div>
                        <p>No messages yet</p>
                        <p className="empty-subtitle">Start chatting with your group!</p>
                    </div>
                )}
                {messages.map((message, index) => {
                    const isMe = message.user_id === currentUserId;
                    const isSystem = message.kind === 'system';
                    const showAvatar = index === 0 || messages[index - 1].user_id !== message.user_id;
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
                                <div className={`message-content ${isSystem ? 'system-message' : 'text'}`}>
                                    {message.content}
                                </div>
                            </div>
                        </div>
                    );
                })}
                <div ref={messagesEndRef} />
            </div>
            <form onSubmit={sendMessage} className="message-input-container">
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
                    disabled={connectionStatus !== 'connected'}
                />
                <button
                    type="submit"
                    className="send-button"
                    disabled={!input.trim() || connectionStatus !== 'connected'}
                    aria-label="Send message"
                >
                    <span className="send-icon">➤</span>
                </button>
            </form>
        </div>
    );
}
