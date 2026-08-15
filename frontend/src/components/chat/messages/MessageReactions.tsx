import type { Message } from '../../../types';
import { reactionByKey } from '../reactionOptions';

interface MessageReactionsProps {
    message: Message;
    /** Called with the reaction key when a chip is toggled. */
    onToggle: (reaction: string) => void;
}

/** The aggregate reaction chips under a message. */
export default function MessageReactions({ message, onToggle }: MessageReactionsProps) {
    if (!message.reactions || message.reactions.length === 0) return null;
    return (
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
                            onClick={() => onToggle(reaction.reaction)}
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
    );
}
