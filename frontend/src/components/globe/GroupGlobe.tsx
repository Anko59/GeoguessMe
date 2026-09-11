import { useEffect, useRef, useState } from 'react';
import type { GroupChallenge, Message } from '../../types';
import Icon from '../ui/Icon';
import Globe from './Globe';
import { useGroupChallenges } from './useGroupChallenges';
import './GroupGlobe.css';

interface GroupGlobeProps {
    groupID: string;
    groupName: string;
    onClose: () => void;
    onChallenge: (message: Message) => void;
}

function locationLabel(item: GroupChallenge) {
    if (item.lat !== undefined && item.long !== undefined) return `${item.lat.toFixed(2)}°, ${item.long.toFixed(2)}°`;
    if (item.location_reveals_at) return `Location hidden until ${new Date(item.location_reveals_at).toLocaleString()}`;
    return 'Guess this challenge to reveal its location';
}

export default function GroupGlobe({ groupID, groupName, onClose, onChallenge }: GroupGlobeProps) {
    const dialog = useRef<HTMLDialogElement>(null);
    const { items, loading, error, refresh } = useGroupChallenges(groupID);
    const [selectedID, setSelectedID] = useState<string | null>(null);
    const selected = items.find((item) => item.photo_id === selectedID);
    const located = items.filter((item) => item.lat !== undefined && item.long !== undefined).length;
    useEffect(() => {
        const element = dialog.current;
        const previousFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        const overflow = document.body.style.overflow;
        document.body.style.overflow = 'hidden';
        element?.showModal();
        return () => {
            element?.close();
            document.body.style.overflow = overflow;
            previousFocus?.focus();
        };
    }, []);
    const openChallenge = (item: GroupChallenge) => {
        onChallenge({
            id: item.photo_id,
            photo_id: item.photo_id,
            group_id: item.group_id,
            user_id: item.user_id,
            username: item.username,
            kind: 'challenge',
            created_at: item.created_at,
            challenge_status: item.status,
            challenge_expires_at: item.expires_at,
        });
    };
    return (
        <dialog
            ref={dialog}
            className="group-globe"
            aria-labelledby="group-globe-title"
            onCancel={(event) => {
                event.preventDefault();
                onClose();
            }}
        >
            <header className="globe-header">
                <div>
                    <p className="globe-eyebrow">{groupName}</p>
                    <h2 id="group-globe-title">Your group's world</h2>
                </div>
                <button type="button" className="globe-close" onClick={onClose} aria-label="Close group globe">
                    <Icon name="close" />
                </button>
            </header>
            <div className="globe-layout">
                <Globe items={items} selectedID={selectedID} onSelect={setSelectedID} />
                <section className="globe-history" aria-label="Group challenges">
                    <div className="globe-history-heading">
                        <h3>Geochallenges</h3>
                        <button type="button" onClick={refresh} disabled={loading}>
                            Refresh
                        </button>
                    </div>
                    <p className="globe-summary">
                        {items.length} {items.length === 1 ? 'challenge' : 'challenges'} · {located} on the globe
                    </p>
                    <p className="globe-hint">
                        Drag to explore, pinch or scroll to zoom. Select a pin or a challenge below.
                    </p>
                    {loading && <p role="status">Loading group challenges…</p>}
                    {error && <p role="alert">{error}</p>}
                    {!loading && !error && items.length === 0 && (
                        <p>No geochallenges yet. Send your first one from the camera!</p>
                    )}
                    {selected && (
                        <div className="globe-selection" aria-live="polite">
                            <strong>{selected.username}'s challenge</strong>
                            <p>{locationLabel(selected)}</p>
                            <button type="button" className="btn btn-primary" onClick={() => openChallenge(selected)}>
                                {selected.status === 'available' ? 'Play challenge' : 'View results'}
                            </button>
                        </div>
                    )}
                    <ul className="globe-challenge-list">
                        {items.map((item) => (
                            <li key={item.photo_id}>
                                <button
                                    type="button"
                                    aria-pressed={selectedID === item.photo_id}
                                    onClick={() => setSelectedID(item.photo_id)}
                                >
                                    <span
                                        className={`globe-location-dot ${item.lat === undefined ? 'is-hidden' : ''}`}
                                        aria-hidden="true"
                                    />
                                    <span>
                                        <strong>{item.username}</strong>
                                        <time dateTime={item.created_at}>
                                            {new Date(item.created_at).toLocaleString()}
                                        </time>
                                        <span>{locationLabel(item)}</span>
                                    </span>
                                </button>
                            </li>
                        ))}
                    </ul>
                </section>
            </div>
        </dialog>
    );
}
