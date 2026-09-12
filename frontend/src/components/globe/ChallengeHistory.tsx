import { useMemo, useState } from 'react';
import type { GroupChallenge } from '../../types';
import { challengeStatusLabel, locationLabel } from './challengeLabels';

interface ChallengeHistoryProps {
    items: GroupChallenge[];
    selectedID: string | null;
    onSelect: (id: string) => void;
}

export default function ChallengeHistory({ items, selectedID, onSelect }: ChallengeHistoryProps) {
    const [query, setQuery] = useState('');
    const [filter, setFilter] = useState('all');
    const [page, setPage] = useState(0);
    const filtered = useMemo(
        () =>
            items.filter(
                (item) =>
                    item.username.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()) &&
                    (filter === 'all' ||
                        (filter === 'available' ? item.status === 'available' : item.lat !== undefined)),
            ),
        [items, query, filter],
    );
    const pageSize = 50;
    const pages = Math.max(1, Math.ceil(filtered.length / pageSize));
    const current = Math.min(page, pages - 1);
    const shown = filtered.slice(current * pageSize, (current + 1) * pageSize);
    return (
        <div className="globe-history-browser">
            <div className="globe-history-filters">
                <input
                    type="search"
                    aria-label="Find a player"
                    placeholder="Find a player"
                    value={query}
                    onChange={(event) => {
                        setQuery(event.target.value);
                        setPage(0);
                    }}
                />
                <select
                    aria-label="Filter challenge list"
                    value={filter}
                    onChange={(event) => {
                        setFilter(event.target.value);
                        setPage(0);
                    }}
                >
                    <option value="all">All challenges</option>
                    <option value="available">Ready to play</option>
                    <option value="located">Revealed locations</option>
                </select>
            </div>
            {filtered.length === 0 && <p role="status">No challenges match. Try another player or filter.</p>}
            <ul className="globe-challenge-list">
                {shown.map((item) => (
                    <li key={item.photo_id}>
                        <button
                            type="button"
                            aria-pressed={selectedID === item.photo_id}
                            onClick={() => onSelect(item.photo_id)}
                        >
                            <span
                                className={`globe-location-dot ${item.lat === undefined ? 'is-hidden' : ''}`}
                                aria-hidden="true"
                            />
                            <span>
                                <strong>{item.username}</strong>
                                <span className="globe-challenge-status">{challengeStatusLabel(item)}</span>
                                <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString()}</time>
                                <span>{locationLabel(item)}</span>
                            </span>
                        </button>
                    </li>
                ))}
            </ul>
            {pages > 1 && (
                <nav className="globe-history-pages" aria-label="Challenge pages">
                    <button type="button" disabled={current === 0} onClick={() => setPage(current - 1)}>
                        Previous
                    </button>
                    <span aria-live="polite">
                        Page {current + 1} of {pages}
                    </span>
                    <button type="button" disabled={current + 1 === pages} onClick={() => setPage(current + 1)}>
                        Next
                    </button>
                </nav>
            )}
        </div>
    );
}
