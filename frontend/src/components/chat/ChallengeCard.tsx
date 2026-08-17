import type { Message } from '../../types';
import ChallengeTimer from './ChallengeTimer';

/** The compact challenge summary card in chat: it reflects the viewer's
 *  challenge state and lets them open the challenge or its results. The card
 *  chrome opens the chat's reply/reaction panel; the dedicated action button
 *  is the element that opens the challenge itself. */
export default function ChallengeCard({
    message,
    isMe,
    onChallengeMessage,
}: {
    message: Message;
    isMe: boolean;
    onChallengeMessage?: (message: Message) => void;
}) {
    // A challenge is resolved once its results are out: the viewer answered it
    // ('guessed') or has viewed the results. 'results' is also what the server
    // sends for the poster, who must keep the 'Challenge sent' label; without
    // that distinction, opening the results of an old challenge flips the card
    // back to a yellow 'New challenge' until refresh.
    const isResolvedChallenge =
        message.challenge_status === 'guessed' || (message.challenge_status === 'results' && !isMe);
    const headerLabel = isResolvedChallenge
        ? 'Resolved challenge'
        : message.challenge_status === 'expired'
          ? 'Challenge expired'
          : isMe
            ? 'Challenge sent'
            : 'New challenge';
    const actionLabel =
        isMe ||
        message.challenge_status === 'results' ||
        message.challenge_status === 'guessed' ||
        message.challenge_status === 'expired'
            ? 'View results'
            : message.challenge_status === 'accepted'
              ? 'Continue challenge'
              : 'Accept challenge';
    return (
        <div
            className={`message-content photo-challenge${isResolvedChallenge ? ' resolved' : ''}`}
            data-photo-id={message.photo_id}
        >
            <span className="challenge-card">
                <span className="challenge-header">
                    <img
                        src={isMe ? '/challenge_sent_icon.png' : '/challenge_received_icon.png'}
                        alt=""
                        className="challenge-icon"
                    />
                    <span>{headerLabel}</span>
                    {message.challenge_expires_at && message.challenge_ttl_seconds ? (
                        <ChallengeTimer
                            expiresAt={message.challenge_expires_at}
                            ttlSeconds={message.challenge_ttl_seconds}
                        />
                    ) : null}
                </span>
                <button type="button" className="start-challenge-btn" onClick={() => onChallengeMessage?.(message)}>
                    {actionLabel}
                </button>
            </span>
        </div>
    );
}
