import { useUserBlocks } from '../../hooks/useUserBlocks';

export default function BlockedUsersManager() {
    const blocks = useUserBlocks();
    return (
        <section className="account-section" aria-labelledby="blocked-users-title">
            <div className="account-section-heading">
                <h2 id="blocked-users-title">Blocked users</h2>
                <p>
                    Blocks hide chat, feed, profiles, and private media in both directions. Group membership and
                    rankings remain unchanged.
                </p>
            </div>
            {blocks.loading && <p role="status">Loading blocked users…</p>}
            {blocks.error && <p role="alert">{blocks.error}</p>}
            {blocks.error && (
                <button
                    className="btn btn-secondary"
                    onClick={blocks.retry}
                    disabled={blocks.loading || Boolean(blocks.pending)}
                >
                    Retry blocked users
                </button>
            )}
            {!blocks.loading && !blocks.error && blocks.items.length === 0 && <p>No blocked users.</p>}
            <ul aria-label="Blocked users">
                {blocks.items.map((item) => (
                    <li key={item.user_id}>
                        <span>{item.username}</span>{' '}
                        <button
                            className="btn btn-secondary"
                            disabled={Boolean(blocks.pending)}
                            onClick={() => void blocks.change(item.user_id, false)}
                            aria-label={`Unblock ${item.username}`}
                        >
                            {blocks.pending === item.user_id ? 'Unblocking…' : 'Unblock'}
                        </button>
                    </li>
                ))}
            </ul>
        </section>
    );
}
