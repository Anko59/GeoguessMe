import { Link } from 'react-router-dom';
import type { Message } from '../../../types';
import Avatar from '../../common/Avatar';
import MessageActions from './MessageActions';
import MessageContent from './MessageContent';
import MessageReactions from './MessageReactions';
import { reactionOptions, type ReactionOption } from '../reactionOptions';

interface MessageRowProps {
    message: Message;
    isMe: boolean;
    isSystem: boolean;
    /** Consecutive same-sender messages are grouped for tighter spacing. */
    grouped: boolean;
    /** Whether this row shows the sender avatar and name. */
    showSender: boolean;
    /** Whether this row's reply/reaction panel is open. */
    actionsOpen: boolean;
    /** ID → message index used to resolve reply targets in O(1). */
    messageIndex: ReadonlyMap<string, Message>;
    reactionPending: string | null;
    reactionOptions?: ReactionOption[];
    /** Toggles this row's reply/reaction panel. */
    onToggleActions: (messageID: string) => void;
    onReply: (message: Message) => void;
    onReaction: (message: Message, reaction: string) => void;
    onChallengeMessage?: (message: Message) => void;
}

/** Interactive descendants keep their own actions: a tap on the challenge CTA,
 *  a full-screen photo, a video, or a profile link must not toggle the panel. */
const NESTED_INTERACTIVE = 'button, a, video, input, textarea, select';

/**
 * The single outer rendering path shared by challenge, text, media, and system
 * messages: avatar, sender, content, actions, and reactions. The content area
 * dispatches to the challenge card or the text/media bubble. Tapping or
 * clicking the message content itself toggles the reply/reaction panel; the
 * panel overlays the message, so opening it never reflows the row.
 */
export default function MessageRow({
    message,
    isMe,
    isSystem,
    grouped,
    showSender,
    actionsOpen,
    messageIndex,
    reactionPending,
    reactionOptions: orderedReactionOptions = reactionOptions,
    onToggleActions,
    onReply,
    onReaction,
    onChallengeMessage,
}: MessageRowProps) {
    const replyTarget = message.reply_to_id ? messageIndex.get(message.reply_to_id) : undefined;

    const handleContentClick = (event: React.MouseEvent<HTMLDivElement>) => {
        if (isSystem) return;
        const target = event.target as HTMLElement;
        // The panel itself and its buttons never re-toggle the panel.
        if (target.closest(`${NESTED_INTERACTIVE}, .message-actions`)) return;
        // Drag-selecting message text is a selection gesture, not a toggle.
        if (window.getSelection()?.toString()) return;
        onToggleActions(message.id);
    };

    const handleRowKeyDown = (event: React.KeyboardEvent<HTMLElement>) => {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        if ((event.target as HTMLElement).closest(NESTED_INTERACTIVE)) return;
        event.preventDefault();
        onToggleActions(message.id);
    };

    const rowClassName = [
        'message-container',
        isMe ? 'own' : 'other',
        isSystem ? 'system' : '',
        grouped ? 'message-grouped' : '',
        actionsOpen ? 'actions-visible' : '',
        'slide-in-up',
    ].join(' ');

    return (
        <div
            data-message-id={message.id}
            className={rowClassName}
            tabIndex={isSystem ? -1 : 0}
            onKeyDown={isSystem ? undefined : handleRowKeyDown}
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
                <div className="message-anchor" onClick={isSystem ? undefined : handleContentClick}>
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
                            visible={actionsOpen}
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
