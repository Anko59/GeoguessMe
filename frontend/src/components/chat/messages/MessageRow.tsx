import { Link } from 'react-router-dom';
import type { Message } from '../../../types';
import Avatar from '../../common/Avatar';
import MessageActions from './MessageActions';
import MessageContent from './MessageContent';
import MessageReactions from './MessageReactions';
import { useMessagePress } from './useMessagePress';
import { reactionOptions, type ReactionOption } from '../reactionOptions';

interface MessageRowProps {
    message: Message;
    isMe: boolean;
    isSystem: boolean;
    /** Consecutive same-sender messages are grouped for tighter spacing. */
    grouped: boolean;
    /** Whether this row shows the sender avatar and name. */
    showSender: boolean;
    actionsVisible: boolean;
    canHover: boolean;
    /** ID → message index used to resolve reply targets in O(1). */
    messageIndex: ReadonlyMap<string, Message>;
    reactionPending: string | null;
    reactionOptions?: ReactionOption[];
    /** A plain tap on this row dismisses actions shown on other rows. */
    onTapDown: (messageID: string) => void;
    onRevealActions: (messageID: string) => void;
    onDismissActions: () => void;
    onReply: (message: Message) => void;
    onReaction: (message: Message, reaction: string) => void;
    onChallengeMessage?: (message: Message) => void;
}

/**
 * The single outer rendering path shared by challenge, text, media, and system
 * messages: avatar, sender, content, actions, and reactions. The content area
 * dispatches to the challenge card or the text/media bubble.
 */
export default function MessageRow({
    message,
    isMe,
    isSystem,
    grouped,
    showSender,
    actionsVisible,
    canHover,
    messageIndex,
    reactionPending,
    reactionOptions: orderedReactionOptions = reactionOptions,
    onTapDown,
    onRevealActions,
    onDismissActions,
    onReply,
    onReaction,
    onChallengeMessage,
}: MessageRowProps) {
    const bindPress = useMessagePress({ onTapDown, onReveal: onRevealActions });
    const press = bindPress(message.id, !isSystem);
    const hover =
        canHover && !isSystem
            ? {
                  onMouseEnter: () => onRevealActions(message.id),
                  onMouseLeave: onDismissActions,
              }
            : {};
    const replyTarget = message.reply_to_id ? messageIndex.get(message.reply_to_id) : undefined;
    const rowClassName = [
        'message-container',
        isMe ? 'own' : 'other',
        isSystem ? 'system' : '',
        grouped ? 'message-grouped' : '',
        actionsVisible ? 'actions-visible' : '',
        'slide-in-up',
    ].join(' ');

    return (
        <div
            data-message-id={message.id}
            className={rowClassName}
            tabIndex={isSystem ? -1 : 0}
            onFocus={press.onFocus}
            onPointerDown={press.onPointerDown}
            onPointerMove={press.onPointerMove}
            onPointerUp={press.onPointerUp}
            onPointerCancel={press.onPointerCancel}
        >
            {!isMe && !isSystem && (
                <div className={`avatar-container${showSender ? '' : ' avatar-placeholder'}`} aria-hidden={!showSender}>
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
                <div className="message-hover-target" {...hover}>
                    <MessageContent
                        message={message}
                        isMe={isMe}
                        isSystem={isSystem}
                        replyTarget={replyTarget}
                        onChallengeMessage={onChallengeMessage}
                    />
                    {!isSystem && (
                        <MessageActions
                            message={message}
                            visible={actionsVisible}
                            reactionPending={reactionPending}
                            reactionOptions={orderedReactionOptions}
                            onReply={() => onReply(message)}
                            onReaction={(reaction) => onReaction(message, reaction)}
                        />
                    )}
                </div>
                {!isSystem && (
                    <MessageReactions message={message} onToggle={(reaction) => onReaction(message, reaction)} />
                )}
            </div>
        </div>
    );
}
