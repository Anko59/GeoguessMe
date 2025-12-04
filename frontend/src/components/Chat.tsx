import { useEffect, useState, useRef } from 'react';
import { getCurrentUserId } from '../utils/userUtils';
import './Chat.css';

interface Message {
    id: string;
    group_id: string;
    user_id: string;
    username?: string;
    avatar?: string;
    content: string;
    created_at: string;
}

interface ChatProps {
    groupID: string;
    onNewMessage?: () => void;
    onChallengeMessage?: (msg: Message) => void;
    messages: Message[];
    setMessages: React.Dispatch<React.SetStateAction<Message[]>>;
    wsRef: React.RefObject<WebSocket | null>;
    myGuesses: string[];
}

export default function Chat({ groupID, onNewMessage, onChallengeMessage, messages, setMessages, wsRef, myGuesses }: ChatProps) {
    const [input, setInput] = useState('');
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const currentUserId = getCurrentUserId();

    // Get WebSocket from parent (GroupView manages the connection)
    useEffect(() => {
        // WebSocket is managed by parent, we just use messages from props
        // Call onNewMessage when new messages arrive
        if (messages.length > 0) {
            onNewMessage?.();
        }
    }, [messages, onNewMessage]);

    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages]);

    const sendMessage = (e: React.FormEvent) => {
        e.preventDefault();
        if (!input.trim() || !wsRef.current) return;

        const message = {
            group_id: groupID,
            user_id: currentUserId,
            content: input.trim(),
        };

        wsRef.current.send(JSON.stringify(message));
        setInput('');
    };

    const isPhotoMessage = (content: string) => {
        return content.startsWith('NEW_PHOTO:');
    };

    const parsePhotoMessage = (content: string) => {
        const parts = content.split(':');
        return {
            photoId: parts[1],
            photoUrl: parts[2]
        };
    };

    const renderMessage = (msg: Message, index: number) => {
        const isMe = msg.user_id === currentUserId;
        const showAvatar = index === 0 || messages[index - 1].user_id !== msg.user_id;

        if (isPhotoMessage(msg.content)) {
            const { photoId } = parsePhotoMessage(msg.content);
            const isCompleted = myGuesses.includes(photoId);

            // Can view details if: sent by you OR completed by you
            const canViewDetails = isMe || isCompleted;

            // Determine which icon to use
            let challengeIcon = '/challenge_received_icon.png';
            let challengeText = 'New Challenge!';
            if (isMe) {
                challengeIcon = '/challenge_sent_icon.png';
                challengeText = 'Challenge Sent!';
            } else if (isCompleted) {
                challengeIcon = '/challenge_completed_icon.png';
                challengeText = 'Challenge Completed!';
            }

            return (
                <div key={index} className={`message-container ${isMe ? 'own' : 'other'} slide-in-up`}>
                    {!isMe && showAvatar && (
                        <div className="avatar-container">
                            <img src={`/avatars/${msg.avatar || 'avatar.png'}`} alt="Avatar" />
                        </div>
                    )}
                    <div className="message-wrapper">
                        {!isMe && <div className="message-username">{msg.username || 'Unknown User'}</div>}
                        <div
                            className={`message-content photo-challenge ${canViewDetails ? 'clickable' : ''}`}
                            onClick={() => canViewDetails && onChallengeMessage?.(msg)}
                        >
                            <div className="challenge-card">
                                <div className="challenge-header">
                                    <img src={challengeIcon} alt="" className="challenge-icon" />
                                    <span>{challengeText}</span>
                                </div>
                                {!isMe && !isCompleted && (
                                    <button
                                        className="start-challenge-btn"
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            onChallengeMessage?.(msg);
                                        }}
                                    >
                                        Accept Challenge
                                    </button>
                                )}
                                {isCompleted && <div className="completed-badge">✓ Done</div>}
                            </div>
                        </div>
                    </div>
                </div>
            );
        }

        const isSystem = msg.user_id === 'SYSTEM';

        return (
            <div key={index} className={`message-container ${isMe ? 'own' : 'other'} ${isSystem ? 'system' : ''} slide-in-up`}>
                {!isMe && !isSystem && (
                    <div className="avatar-container">
                        <img src={`/avatars/${msg.avatar || 'avatar.png'}`} alt="Avatar" className="avatar" />
                    </div>
                )}
                <div className="message-wrapper">
                    {!isMe && !isSystem && <div className="message-username">{msg.username || 'Unknown User'}</div>}
                    <div className={`message-content ${isSystem ? 'system-message' : 'text'}`}>
                        {msg.content}
                    </div>
                </div>
            </div>
        );
    };

    return (
        <div className="chat-container">
            <div className="messages-list">
                {messages.length === 0 && (
                    <div className="empty-state">
                        <div className="empty-icon">💬</div>
                        <p>No messages yet</p>
                        <p className="empty-subtitle">Start chatting with your group!</p>
                    </div>
                )}
                {messages.map((msg, i) => renderMessage(msg, i))}
                <div ref={messagesEndRef} />
            </div>

            <form onSubmit={sendMessage} className="message-input-container">
                <input
                    type="text"
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    placeholder="Type a message..."
                    className="message-input"
                />
                <button
                    type="submit"
                    className="send-button"
                    disabled={!input.trim()}
                >
                    <span className="send-icon">➤</span>
                </button>
            </form>
        </div>
    );
}
