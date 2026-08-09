import type { Message } from '../../../types';
import { reactionOptions } from '../reactionOptions';

interface MessageActionsProps {
    message: Message;
    /** Whether the row's actions are revealed (tab-index 0 for keyboard users). */
    visible: boolean;
    /** The in-flight reaction key (`messageID:reaction`), if any. */
    reactionPending: string | null;
    onReply: () => void;
    onReaction: (reaction: string) => void;
}

/** The reply button and the reaction picker revealed on hover, long press,
 *  swipe, or keyboard focus. */
export default function MessageActions({
    message,
    visible,
    reactionPending,
    onReply,
    onReaction,
}: MessageActionsProps) {
    const tabIndex = visible ? 0 : -1;
    return (
        <div className="message-actions" aria-label="Message actions">
            <button
                type="button"
                className="reply-action"
                tabIndex={tabIndex}
                onClick={onReply}
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
                            tabIndex={tabIndex}
                            onClick={() => onReaction(reaction)}
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
}
