import type { Message } from '../../../types';

function formatMessageTime(iso: string): string {
    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) return iso;
    return date.toLocaleString(undefined, { dateStyle: 'short', timeStyle: 'short' });
}

interface ReplyPreviewProps {
    message: Message;
    /** The message this message replies to, resolved from the message index. */
    replyTarget?: Message;
}

/** The quoted context shown above a message that replies to another message.
 *  A missing or deleted target degrades to "Message unavailable". */
export default function ReplyPreview({ message, replyTarget }: ReplyPreviewProps) {
    if (!message.reply_to_id) return null;
    return (
        <div className="reply-context">
            <strong>{replyTarget?.username || 'Original message'}</strong>
            <span>
                {replyTarget
                    ? replyTarget.kind === 'challenge'
                        ? `Message sent at ${formatMessageTime(replyTarget.created_at)}`
                        : replyTarget.content || 'Message unavailable'
                    : 'Message unavailable'}
            </span>
        </div>
    );
}
