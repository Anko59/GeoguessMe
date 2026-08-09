import type { Message } from '../../../types';
import ChatAttachment from '../ChatAttachment';
import ChallengeCard from '../ChallengeCard';
import ReplyPreview from './ReplyPreview';

interface MessageContentProps {
    message: Message;
    isMe: boolean;
    isSystem: boolean;
    /** The resolved reply target (undefined when missing or deleted). */
    replyTarget?: Message;
    onChallengeMessage?: (message: Message) => void;
}

/** The message body: a challenge card, or the bubble with its reply preview,
 *  media attachment, and caption. */
export default function MessageContent({
    message,
    isMe,
    isSystem,
    replyTarget,
    onChallengeMessage,
}: MessageContentProps) {
    if (message.kind === 'challenge') {
        return <ChallengeCard message={message} isMe={isMe} onChallengeMessage={onChallengeMessage} />;
    }
    return (
        <div
            className={`message-content ${isSystem ? 'system-message' : message.kind === 'media' ? 'media-message' : 'text'}`}
        >
            <ReplyPreview message={message} replyTarget={replyTarget} />
            {message.kind === 'media' && message.media_id && message.media_type && (
                <ChatAttachment mediaID={message.media_id} mediaType={message.media_type} />
            )}
            {message.content && <p className="message-caption">{message.content}</p>}
        </div>
    );
}
