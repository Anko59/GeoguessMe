import { useCallback, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { getAPIErrorMessage, groupsAPI } from '../../api';
import type { GroupInbox } from '../../types';

function groupInitial(name: string): string {
    return name.trim().slice(0, 1).toUpperCase() || '?';
}

export default function GroupRail() {
    const [items, setItems] = useState<GroupInbox[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');

    const load = useCallback((signal?: AbortSignal) => {
        setLoading(true);
        setError('');
        return groupsAPI
            .inbox(signal)
            .then(setItems)
            .catch((requestError: unknown) => {
                if (!signal?.aborted) setError(getAPIErrorMessage(requestError, 'Unable to load your group inbox.'));
            })
            .finally(() => {
                if (!signal?.aborted) setLoading(false);
            });
    }, []);

    useEffect(() => {
        const controller = new AbortController();
        queueMicrotask(() => void load(controller.signal));
        return () => controller.abort();
    }, [load]);

    if (loading) {
        return (
            <section className="group-rail" aria-label="Group inbox" aria-busy="true">
                <p className="group-rail-status" role="status">
                    Loading your groups…
                </p>
            </section>
        );
    }

    if (error) {
        return (
            <section className="group-rail" aria-label="Group inbox">
                <p className="group-rail-status" role="alert">
                    {error}
                </p>
                <button className="feed-text-button" onClick={() => void load()}>
                    Retry
                </button>
            </section>
        );
    }

    if (items.length === 0) {
        return (
            <section className="group-rail" aria-label="Group inbox">
                <p className="group-rail-status">No groups yet.</p>
                <Link to="/groups" className="feed-text-button">
                    Find your groups →
                </Link>
            </section>
        );
    }

    return (
        <nav className="group-rail" aria-label="Group inbox">
            {items.map((group) => (
                <Link key={group.id} to={`/group/${group.id}`} className="group-rail-item">
                    <span className="group-rail-avatar" aria-hidden="true">
                        {groupInitial(group.name)}
                    </span>
                    <span className="group-rail-copy">
                        <strong>{group.name}</strong>
                        {group.latest_message ? (
                            <span>
                                {group.latest_message.username} · {group.latest_message.kind}
                            </span>
                        ) : (
                            <span>No messages yet</span>
                        )}
                    </span>
                    {group.unread_count > 0 && (
                        <span className="group-rail-unread" aria-label={`${group.unread_count} unread messages`}>
                            {group.unread_count > 99 ? '99+' : group.unread_count}
                        </span>
                    )}
                </Link>
            ))}
        </nav>
    );
}
