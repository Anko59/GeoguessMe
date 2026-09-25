import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import FeedCard, { FeedResultsFooter } from './FeedCard';
import FeedComposer from './FeedComposer';
import { useFeed } from './useFeed';
import AuthenticatedPageShell from '../../components/layout/AuthenticatedPageShell';
import FeedGame from './FeedGame';
import './FeedForms.css';
import './Feed.css';
import './FeedAudience.css';

function FeedPage() {
    const feed = useFeed();
    const navigate = useNavigate();
    const [composing, setComposing] = useState(false);
    return (
        <AuthenticatedPageShell className="public-feed">
            <main className="feed-layout">
                <div className="feed-column">
                    <section className="feed-heading">
                        <div>
                            <p className="feed-eyebrow">Community challenges</p>
                            <h1>Explore the world</h1>
                            <p>Open a photo. Guess the place. Make it clear in your feed.</p>
                        </div>
                        <button className="btn btn-primary" onClick={() => setComposing(true)}>
                            + Post a challenge
                        </button>
                    </section>
                    <div className="feed-section-label">
                        <h2>Latest challenges</h2>
                    </div>
                    {feed.error && (
                        <div className="feed-empty" role="alert">
                            <p>{feed.error}</p>
                            <button
                                className="btn btn-secondary"
                                disabled={feed.pending}
                                onClick={() => void feed.load(feed.cursor)}
                            >
                                Try again
                            </button>
                        </div>
                    )}
                    {!feed.loaded && !feed.error && (
                        <div className="feed-empty" role="status">
                            Finding your next adventure…
                        </div>
                    )}
                    {feed.loaded && feed.items.length === 0 && (
                        <section className="feed-empty">
                            <img src="/globe_icon.png" alt="" />
                            <h2>The world is waiting for your first post</h2>
                            <p>Take a photo now and let your device add the location.</p>
                            <button className="btn btn-secondary" onClick={() => setComposing(true)}>
                                Post a geo challenge
                            </button>
                        </section>
                    )}
                    {feed.items.map((post) => (
                        <FeedCard key={post.id} post={post} onUpdate={feed.update} onRemove={feed.remove} />
                    ))}
                    {feed.cursor && (
                        <button
                            className="btn btn-secondary feed-load-more"
                            disabled={feed.pending}
                            onClick={() => void feed.load(feed.cursor)}
                        >
                            {feed.pending ? 'Loading…' : 'More adventures'}
                        </button>
                    )}
                </div>
                <aside className="feed-sidebar">
                    <img src="/globe_icon.png" alt="" />
                    <p className="feed-eyebrow">A little curiosity goes a long way</p>
                    <h2>Look. Guess. Connect.</h2>
                    <ol>
                        <li>
                            <strong>Find a mystery</strong>
                            <span>Every blurred photo is a place to discover.</span>
                        </li>
                        <li>
                            <strong>Guess the place</strong>
                            <span>Open the photo and make one guess. Any guess reveals it.</span>
                        </li>
                        <li>
                            <strong>Share the moment</strong>
                            <span>Reveal the photo, leave a heart, join the conversation.</span>
                        </li>
                    </ol>
                    <Link to="/groups">Looking for your friends? Visit your groups →</Link>
                </aside>
            </main>
            {composing && (
                <FeedComposer
                    onClose={() => setComposing(false)}
                    onPublished={(postID) => {
                        setComposing(false);
                        navigate(`/feed/${postID}`);
                    }}
                />
            )}
        </AuthenticatedPageShell>
    );
}

function FeedResultsRoute({ id }: { id: string }) {
    const feed = useFeed(id);
    const post = feed.items[0];
    const navigate = useNavigate();

    return (
        <AuthenticatedPageShell className="public-feed">
            <main className="feed-result-route">
                {!feed.loaded && !feed.error && <p role="status">Loading challenge results…</p>}
                {feed.error && (
                    <div role="alert">
                        <p>{feed.error}</p>
                        <button className="btn btn-secondary" disabled={feed.pending} onClick={() => void feed.load()}>
                            Try again
                        </button>
                    </div>
                )}
                {feed.loaded && !post && !feed.error && (
                    <section className="feed-empty">
                        <h1>Challenge unavailable</h1>
                        <p>This challenge is no longer available.</p>
                        <Link className="btn btn-secondary" to="/feed">
                            Back to feed
                        </Link>
                    </section>
                )}
                {post && (
                    <FeedGame
                        id={post.id}
                        isOwner={post.is_owner}
                        openResultsDirectly={post.is_owner || post.resolved}
                        restoreFocus={() => {}}
                        onClose={() => navigate('/feed')}
                        onResolved={() => feed.update({ id: post.id, resolved: true })}
                        resultsFooter={
                            post.is_owner || post.resolved ? (
                                <FeedResultsFooter post={post} onUpdate={feed.update} />
                            ) : undefined
                        }
                    />
                )}
            </main>
        </AuthenticatedPageShell>
    );
}

export default function Feed() {
    const { id } = useParams();
    return id ? <FeedResultsRoute key={id} id={id} /> : <FeedPage />;
}
