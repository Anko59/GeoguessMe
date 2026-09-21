import { useCallback, useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { PublicChallenge } from '../../types';
import Avatar from '../../components/common/Avatar';
import FeedImage from './FeedImage';
import FeedGame from './FeedGame';
import FeedComments from './FeedComments';
import FeedShare from './FeedShare';
import { useFeedActions } from './useFeed';

function FeedSocialSummary({
    post,
    onReact,
    onComment,
    commentsOpen,
    commentsID,
}: {
    post: PublicChallenge;
    onReact: () => void;
    onComment?: () => void;
    commentsOpen?: boolean;
    commentsID?: string;
}) {
    const actions = useFeedActions(post.id);
    return (
        <div className="feed-social-actions">
            <button
                className={`feed-reaction ${post.reacted ? 'is-liked' : ''}`}
                aria-label={post.reacted ? 'Unlike challenge' : 'Like challenge'}
                aria-pressed={post.reacted}
                disabled={actions.pending}
                onClick={() => void onReact()}
            >
                <img src="/reactions/like.png" alt="" aria-hidden="true" className="feed-action-icon" />
                <span>
                    {post.reaction_count} {post.reaction_count === 1 ? 'like' : 'likes'}
                </span>
            </button>
            {onComment ? (
                <button
                    className="feed-icon-button"
                    aria-label={`${post.comment_count} ${post.comment_count === 1 ? 'comment' : 'comments'}`}
                    aria-expanded={commentsOpen}
                    aria-controls={commentsID}
                    onClick={onComment}
                >
                    <img src="/chat_bubbl_icon.png" alt="" aria-hidden="true" className="feed-action-icon" />
                    {post.comment_count} {post.comment_count === 1 ? 'comment' : 'comments'}
                </button>
            ) : (
                <span
                    className="feed-social-count"
                    aria-label={`${post.comment_count} ${post.comment_count === 1 ? 'comment' : 'comments'}`}
                >
                    {post.comment_count} {post.comment_count === 1 ? 'comment' : 'comments'}
                </span>
            )}
            <FeedShare id={post.id} username={post.username} />
            {actions.error && (
                <p role="alert" className="error-message">
                    {actions.error}
                </p>
            )}
        </div>
    );
}

function FeedResultsFooter({
    post,
    onUpdate,
}: {
    post: PublicChallenge;
    onUpdate: (post: Partial<PublicChallenge> & { id: string }) => void;
}) {
    const actions = useFeedActions(post.id);
    const react = async () => {
        if (await actions.react(!post.reacted))
            onUpdate({
                id: post.id,
                reacted: !post.reacted,
                reaction_count: Math.max(0, post.reaction_count + (post.reacted ? -1 : 1)),
            });
    };
    return (
        <section className="feed-results-footer" aria-label="Challenge discussion">
            <FeedSocialSummary post={post} onReact={() => void react()} />
            <FeedComments
                id={post.id}
                onCountChange={(delta) =>
                    onUpdate({ id: post.id, comment_count: Math.max(0, post.comment_count + delta) })
                }
            />
        </section>
    );
}

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
    const mediaRef = useRef<HTMLDivElement>(null);
    const restorePlayFocus = useCallback(() => {
        playButton.current?.focus();
        if (!playButton.current) mediaRef.current?.focus();
    }, []);
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
    function openChallenge() {
        setPlaying(true);
    }
    function onMediaKeyDown(event: KeyboardEvent<HTMLDivElement>) {
        if (!revealed || (event.key !== 'Enter' && event.key !== ' ')) return;
        event.preventDefault();
        openChallenge();
    }
    const resultsFooter: ReactNode = revealed ? <FeedResultsFooter post={post} onUpdate={onUpdate} /> : undefined;

    return (
        <article className="feed-card" aria-label={`Geo challenge by ${post.username}`}>
            <header className="feed-card-header">
                <Avatar userID={post.user_id} avatar={post.avatar} username={post.username} className="feed-avatar" />
                <div className="feed-author">
                    <strong>{post.username}</strong>
                    <Link to={`/feed/${post.id}`}>
                        <time dateTime={post.created_at}>
                            {new Date(post.created_at).toLocaleDateString(undefined, {
                                month: 'short',
                                day: 'numeric',
                            })}
                        </time>{' '}
                        · {post.audience === 'friends' ? 'Friends challenge' : 'Public challenge'}
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
            <div
                ref={mediaRef}
                className={`feed-media${revealed ? ' feed-media--interactive' : ''}`}
                role={revealed ? 'button' : undefined}
                tabIndex={revealed ? 0 : undefined}
                aria-label={revealed ? 'Open challenge results' : undefined}
                onClick={revealed ? openChallenge : undefined}
                onKeyDown={onMediaKeyDown}
            >
                <FeedImage id={post.id} revealed={revealed} />
                {!revealed && (
                    <div className="feed-reveal-overlay">
                        <h2>Where was this taken?</h2>
                        <p>See the photo, then guess to reveal it here.</p>
                        <button ref={playButton} className="btn btn-primary" onClick={openChallenge}>
                            Play challenge
                        </button>
                    </div>
                )}
                {revealed && (
                    <span className="feed-revealed-badge">{post.is_owner ? 'Your challenge' : '✓ Revealed'}</span>
                )}
            </div>
            <div className="feed-card-body">
                <FeedSocialSummary
                    post={post}
                    onReact={() => void react()}
                    onComment={() => setCommentsOpen((open) => !open)}
                    commentsOpen={commentsOpen}
                    commentsID={commentsID}
                />
                {post.caption && (
                    <p className="feed-caption">
                        <strong>{post.username}</strong> {post.caption}
                    </p>
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
                    alreadyResolved={post.is_owner || post.resolved}
                    restoreFocus={restorePlayFocus}
                    onClose={() => setPlaying(false)}
                    onResolved={() => onUpdate({ id: post.id, resolved: true })}
                    resultsFooter={resultsFooter}
                />
            )}
        </article>
    );
}
