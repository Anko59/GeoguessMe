import { useCallback, useId, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { PublicChallenge } from '../../types';
import FeedImage from './FeedImage';
import FeedGame from './FeedGame';
import FeedComments from './FeedComments';
import FeedShare from './FeedShare';
import { useFeedActions } from './useFeed';

export default function FeedCard({
    post,
    onUpdate,
    onRemove,
}: {
    post: PublicChallenge;
    onUpdate: (post: Partial<PublicChallenge> & { id: string }) => void;
    onRemove: (id: string) => void;
}) {
    const commentsID = useId();
    const playButton = useRef<HTMLButtonElement>(null);
    const restorePlayFocus = useCallback(() => playButton.current?.focus(), []);
    const [playing, setPlaying] = useState(false);
    const [commentsOpen, setCommentsOpen] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const actions = useFeedActions(post.id);
    const revealed = post.is_owner || post.resolved;
    async function react() {
        if (await actions.react(!post.reacted))
            onUpdate({
                id: post.id,
                reacted: !post.reacted,
                reaction_count: Math.max(0, post.reaction_count + (post.reacted ? -1 : 1)),
            });
    }
    async function remove() {
        if (await actions.remove()) onRemove(post.id);
    }
    return (
        <article className="feed-card" aria-label={`Geo challenge by ${post.username}`}>
            <header className="feed-card-header">
                <span className="feed-avatar" aria-hidden="true">
                    {post.username.slice(0, 1).toUpperCase()}
                </span>
                <div className="feed-author">
                    <strong>{post.username}</strong>
                    <Link to={`/feed/${post.id}`}>
                        <time dateTime={post.created_at}>
                            {new Date(post.created_at).toLocaleDateString(undefined, {
                                month: 'short',
                                day: 'numeric',
                            })}
                        </time>{' '}
                        · Public challenge
                    </Link>
                </div>
                {post.is_owner && (
                    <button className="feed-text-button" onClick={() => setConfirmDelete(true)}>
                        Delete post
                    </button>
                )}
            </header>
            {confirmDelete && (
                <div className="feed-delete-confirm">
                    <p>Delete this challenge and its comments?</p>
                    <button
                        className="btn btn-secondary"
                        onClick={() => setConfirmDelete(false)}
                        disabled={actions.pending}
                    >
                        Keep post
                    </button>
                    <button className="btn btn-danger" disabled={actions.pending} onClick={() => void remove()}>
                        Confirm delete
                    </button>
                </div>
            )}
            <div className="feed-media">
                <FeedImage id={post.id} revealed={revealed} />
                {!revealed && (
                    <div className="feed-reveal-overlay">
                        <h2>Where was this taken?</h2>
                        <p>See the photo, then guess to reveal it here.</p>
                        <button ref={playButton} className="btn btn-primary" onClick={() => setPlaying(true)}>
                            Play challenge
                        </button>
                    </div>
                )}
                {revealed && (
                    <span className="feed-revealed-badge">{post.is_owner ? 'Your challenge' : '✓ Revealed'}</span>
                )}
            </div>
            <div className="feed-card-body">
                <div className="feed-social-actions">
                    <button
                        className={`feed-reaction ${post.reacted ? 'is-liked' : ''}`}
                        aria-label={post.reacted ? 'Unlike challenge' : 'Like challenge'}
                        aria-pressed={post.reacted}
                        disabled={actions.pending}
                        onClick={() => void react()}
                    >
                        <span aria-hidden="true">{post.reacted ? '♥' : '♡'}</span>
                        <span>
                            {post.reaction_count} {post.reaction_count === 1 ? 'like' : 'likes'}
                        </span>
                    </button>
                    <button
                        className="feed-text-button"
                        aria-expanded={commentsOpen}
                        aria-controls={commentsID}
                        onClick={() => setCommentsOpen((open) => !open)}
                    >
                        {post.comment_count} {post.comment_count === 1 ? 'comment' : 'comments'}
                    </button>
                    <FeedShare id={post.id} username={post.username} />
                </div>
                {post.caption && (
                    <p className="feed-caption">
                        <strong>{post.username}</strong> {post.caption}
                    </p>
                )}
                {post.resolved && !post.is_owner && (
                    <button ref={playButton} className="feed-text-button" onClick={() => setPlaying(true)}>
                        View your result
                    </button>
                )}
                {!revealed && !commentsOpen && (
                    <p className="feed-spoiler-note">Comments may contain clues or spoilers.</p>
                )}
                {actions.error && (
                    <p role="alert" className="error-message">
                        {actions.error}
                    </p>
                )}
                <div id={commentsID}>
                    {commentsOpen && (
                        <FeedComments
                            id={post.id}
                            onCountChange={(delta) =>
                                onUpdate({ id: post.id, comment_count: Math.max(0, post.comment_count + delta) })
                            }
                        />
                    )}
                </div>
            </div>
            {playing && (
                <FeedGame
                    id={post.id}
                    alreadyResolved={post.resolved}
                    restoreFocus={restorePlayFocus}
                    onClose={() => setPlaying(false)}
                    onResolved={() => onUpdate({ id: post.id, resolved: true })}
                />
            )}
        </article>
    );
}
