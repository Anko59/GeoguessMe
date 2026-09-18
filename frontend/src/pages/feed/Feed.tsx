import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import FeedCard from './FeedCard';
import FeedComposer from './FeedComposer';
import { useFeed } from './useFeed';
import './FeedForms.css';
import './Feed.css';

function FeedPage({ id }: { id?: string }) {
    const feed = useFeed(id);
    const navigate = useNavigate();
    const [composing, setComposing] = useState(false);
    return (
        <div className="public-feed">
            <header className="feed-topbar">
                <Link to="/feed" className="feed-brand">
                    <img src="/logo.png" alt="" />
                    <span>GeoGuessMe</span>
                </Link>
                <nav aria-label="Main navigation">
                    <Link to="/feed" aria-current="page">
                        Explore
                    </Link>
                    <Link to="/groups">My groups</Link>
                    <Link to="/profile">Profile</Link>
                    <Link to="/settings">Settings</Link>
                </nav>
            </header>
            <main className="feed-layout">
                <div className="feed-column">
                    <section className="feed-heading">
                        <div>
                            <p className="feed-eyebrow">Community challenges</p>
                            <h1>{id ? 'Geo challenge' : 'Explore the world'}</h1>
                            <p>Open a photo. Guess the place. Make it clear in your feed.</p>
                        </div>
                        <button className="btn btn-primary" onClick={() => setComposing(true)}>
                            + Post a challenge
                        </button>
                    </section>
                    <div className="feed-section-label">
                        <h2>{id ? 'Shared with the community' : 'Latest challenges'}</h2>
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
                            <h2>
                                {id ? 'This challenge has been removed' : 'The world is waiting for your first post'}
                            </h2>
                            <p>Share a photo, pin its location, and let the community find it.</p>
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
                    {id && (
                        <Link className="feed-back-link" to="/feed">
                            Explore more challenges →
                        </Link>
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
                            <strong>Put a pin in it</strong>
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
        </div>
    );
}

export default function Feed() {
    const { id } = useParams();
    return <FeedPage key={id ?? 'all'} id={id} />;
}
