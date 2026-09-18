import { useId, useState, type FormEvent } from 'react';
import { useFeedActions, usePublicComments } from './useFeed';

export default function FeedComments({ id, onCountChange }: { id: string; onCountChange: (delta: number) => void }) {
    const inputID = useId();
    const comments = usePublicComments(id);
    const actions = useFeedActions(id);
    const [content, setContent] = useState('');
    async function submit(event: FormEvent) {
        event.preventDefault();
        if (!comments.loaded || !content.trim()) return;
        const comment = await actions.comment(content.trim());
        if (comment) {
            comments.add(comment);
            setContent('');
            onCountChange(1);
        }
    }
    async function remove(commentID: string) {
        if (await actions.removeComment(commentID)) {
            comments.remove(commentID);
            onCountChange(-1);
        }
    }
    return (
        <section className="feed-comments" aria-label="Comments">
            {comments.pending && <p role="status">Loading comments…</p>}
            {comments.error && (
                <p role="alert">
                    {comments.error} <button onClick={() => void comments.load(comments.cursor)}>Retry comments</button>
                </p>
            )}
            {comments.loaded && comments.items.length === 0 && (
                <p className="feed-note">No comments yet. Start the conversation.</p>
            )}
            <ul>
                {comments.items.map((comment) => (
                    <li key={comment.id}>
                        <div>
                            <strong>{comment.username}</strong>
                            <p>{comment.content}</p>
                            <time dateTime={comment.created_at}>
                                {new Date(comment.created_at).toLocaleDateString()}
                            </time>
                        </div>
                        {comment.can_delete && (
                            <button
                                className="feed-text-button"
                                aria-label={`Delete comment by ${comment.username}`}
                                disabled={actions.pending}
                                onClick={() => void remove(comment.id)}
                            >
                                Delete
                            </button>
                        )}
                    </li>
                ))}
            </ul>
            {comments.cursor && (
                <button
                    className="feed-text-button"
                    disabled={comments.pending}
                    onClick={() => void comments.load(comments.cursor)}
                >
                    Older comments
                </button>
            )}
            <form className="feed-comment-form" onSubmit={submit}>
                <label htmlFor={inputID}>Add a comment</label>
                <textarea
                    id={inputID}
                    value={content}
                    maxLength={1000}
                    required
                    placeholder="What did you think of this place?"
                    disabled={actions.pending || !comments.loaded}
                    onChange={(e) => setContent(e.target.value)}
                />
                {actions.error && (
                    <p role="alert" className="error-message">
                        {actions.error}
                    </p>
                )}
                <button
                    className="btn btn-secondary"
                    type="submit"
                    disabled={actions.pending || !comments.loaded || !content.trim()}
                >
                    {actions.pending ? 'Saving…' : 'Post comment'}
                </button>
            </form>
        </section>
    );
}
