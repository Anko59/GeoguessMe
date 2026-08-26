import { useEffect, useRef } from 'react';
import type { Message } from '../../../types';
import { type ReactionOption } from '../reactionOptions';

interface MessageActionsProps {
    message: Message;
    /** Whether the row's actions are revealed (tab-index 0 for keyboard users). */
    visible: boolean;
    /** The in-flight reaction key (`messageID:reaction`), if any. */
    reactionPending: string | null;
    onReply: () => void;
    onReaction: (reaction: string) => void;
    reactionOptions: ReactionOption[];
}

/** The reply button and the reaction picker, opened by tapping or clicking the
 *  message itself. Rendered as an overlay above the reaction chips: the
 *  reactions sit in one horizontally scrollable row, so the panel never
 *  overflows the viewport and never changes the message's layout. */
export default function MessageActions({
    message,
    visible,
    reactionPending,
    onReply,
    onReaction,
    reactionOptions,
}: MessageActionsProps) {
    const panelRef = useRef<HTMLDivElement>(null);
    const tabIndex = visible ? 0 : -1;

    // The panel overlays the chat below its message; keep every part of it
    // inside the visible scroll region when it opens near a list edge.
    useEffect(() => {
        if (visible) panelRef.current?.scrollIntoView({ block: 'nearest' });
    }, [visible]);

    return (
        <div ref={panelRef} className="message-actions" role="group" aria-label="Message actions">
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
