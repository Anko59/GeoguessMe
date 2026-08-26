import { useEffect, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import type { Message, ReactionUsage } from '../../types';
import { sortReactionOptions } from './reactionOptions';
import Composer from './composer/Composer';
import MessageRow from './messages/MessageRow';
import { useMessageIndex } from './messages/messageIndex';
import { useInfiniteScroll } from './useInfiniteScroll';
import './Chat.css';
import './ChatActions.css';
import './ChatHistory.css';

interface ChatProps {
    messages: Message[];
    wsRef: React.RefObject<WebSocket | null>;
    currentUserId: string;
    groupID: string;
    connectionStatus?: 'connecting' | 'connected' | 'offline';
    onChallengeMessage?: (message: Message) => void;
    onMessageUpdated?: (message: Message) => void;
    /** Load the page of messages older than the oldest rendered one. */
    onLoadOlder?: () => void;
    hasMoreOlder?: boolean;
    loadingOlder?: boolean;
}

export default function Chat({
    messages,
    wsRef,
    currentUserId,
    groupID,
    connectionStatus = 'offline',
    onChallengeMessage,
    onMessageUpdated,
    onLoadOlder,
    hasMoreOlder = false,
    loadingOlder = false,
}: ChatProps) {
    const [replyingTo, setReplyingTo] = useState<Message | null>(null);
    const [actionsMessageID, setActionsMessageID] = useState<string | null>(null);
    const [reactionPending, setReactionPending] = useState<string | null>(null);
    const [reactionError, setReactionError] = useState('');
    const [reactionUsage, setReactionUsage] = useState<ReactionUsage[]>([]);
    const messagesEndRef = useRef<HTMLDivElement>(null);
    const messagesListRef = useRef<HTMLDivElement>(null);
    const messageIndex = useMessageIndex(messages);
    const orderedReactionOptions = sortReactionOptions(reactionUsage);

    const focusMessageRow = (messageID: string) => {
        document.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(messageID)}"]`)?.focus();
    };

    // The panel closes on any pointer down outside the row that owns it and on
    // Escape, like a popover. A tap on another row's own controls replaces it
    // through that row's toggle instead.
    useEffect(() => {
        if (!actionsMessageID) return undefined;
        const dismissActions = (event: PointerEvent) => {
            const row = (event.target as HTMLElement).closest('[data-message-id]');
            if (row?.getAttribute('data-message-id') !== actionsMessageID) setActionsMessageID(null);
        };
        const dismissOnEscape = (event: KeyboardEvent) => {
            if (event.key === 'Escape') setActionsMessageID(null);
        };
        document.addEventListener('pointerdown', dismissActions);
        document.addEventListener('keydown', dismissOnEscape);
        return () => {
            document.removeEventListener('pointerdown', dismissActions);
            document.removeEventListener('keydown', dismissOnEscape);
        };
    }, [actionsMessageID]);

    useEffect(() => {
        let active = true;
        void api
            .get<ReactionUsage[]>('/group/reaction-usage', { params: { group_id: groupID } })
            .then((response) => {
                if (active) setReactionUsage(response.data);
            })
            .catch(() => {
                // The curated order remains a complete and usable fallback.
            });
        return () => {
            active = false;
        };
    }, [groupID]);

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
            // The action completed from inside the panel; return focus to the
            // message so keyboard users keep their place in the chat.
            focusMessageRow(message.id);
        } catch (requestError: unknown) {
            setReactionError(getAPIErrorMessage(requestError, 'Unable to save reaction'));
        } finally {
            setReactionPending(null);
        }
    };

    const { onScroll: handleMessagesScroll } = useInfiniteScroll(
        messagesListRef,
        messagesEndRef,
        messages,
        loadingOlder,
        hasMoreOlder,
        onLoadOlder,
    );

    return (
        <div className="chat-container">
            <div className={`chat-status ${connectionStatus}`} role="status" aria-live="polite">
                {connectionStatus === 'connected'
                    ? 'Connected'
                    : connectionStatus === 'connecting'
                      ? 'Connecting…'
                      : 'Offline — retrying'}
            </div>
            <div className="messages-list" ref={messagesListRef} onScroll={handleMessagesScroll}>
                {loadingOlder && (
                    <div className="messages-loading-older" aria-live="polite">
                        Loading older messages…
                    </div>
                )}
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
                    return (
                        <MessageRow
                            key={message.id}
                            message={message}
                            isMe={isMe}
                            isSystem={isSystem}
                            grouped={sameSenderGroup}
                            showSender={showSender}
                            actionsOpen={actionsMessageID === message.id}
                            messageIndex={messageIndex}
                            reactionPending={reactionPending}
                            reactionOptions={orderedReactionOptions}
                            onToggleActions={(messageID) =>
                                setActionsMessageID((current) => (current === messageID ? null : messageID))
                            }
                            onReply={(target) => {
                                setReplyingTo(target);
                                setActionsMessageID(null);
                                focusMessageRow(target.id);
                            }}
                            onReaction={handleReaction}
                            onChallengeMessage={onChallengeMessage}
                        />
                    );
                })}
                <div ref={messagesEndRef} />
            </div>
            {reactionError && (
                <p className="chat-reaction-error" role="alert">
                    {reactionError}
                </p>
            )}
            <Composer
                wsRef={wsRef}
                groupID={groupID}
                connectionStatus={connectionStatus}
                replyingTo={replyingTo}
                onCancelReply={() => setReplyingTo(null)}
            />
        </div>
    );
}
