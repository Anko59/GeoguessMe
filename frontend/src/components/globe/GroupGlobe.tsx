import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react';
import type { GroupChallenge, Message } from '../../types';
import Icon from '../ui/Icon';
import Globe from './Globe';
import ChallengeHistory from './ChallengeHistory';
import { challengeStatusLabel, locationLabel } from './challengeLabels';
import { useGroupChallenges } from './useGroupChallenges';
import './GroupGlobe.css';

interface GroupGlobeProps {
    groupID: string;
    groupName: string;
    /** Changes when a realtime challenge is published for this group. */
    challengeRevision?: string;
    onClose: () => void;
    onChallenge: (message: Message) => void;
}

export default function GroupGlobe({
    groupID,
    groupName,
    challengeRevision = '',
    onClose,
    onChallenge,
}: GroupGlobeProps) {
    const dialog = useRef<HTMLDialogElement>(null);
    const selection = useRef<HTMLDivElement>(null);
    const { items, loading, error, refresh } = useGroupChallenges(groupID);
    const [selectedID, setSelectedID] = useState<string | null>(null);
    const [listOpen, setListOpen] = useState(false);
    const dragStartY = useRef<number | null>(null);
    const suppressGrabberClick = useRef(false);
    const previousChallengeRevision = useRef(challengeRevision);
    const selected = items.find((item) => item.photo_id === selectedID);
    const located = items.filter((item) => item.lat !== undefined && item.long !== undefined).length;
    const selectFromGlobe = (id: string) => {
        setSelectedID(id);
        setListOpen(true);
    };
    const toggleList = () => setListOpen((open) => !open);
    useEffect(() => {
        if (previousChallengeRevision.current === challengeRevision) return;
        previousChallengeRevision.current = challengeRevision;
        // A challenge can be published by another member while this dialog is
        // open. The chat socket is the invalidation signal; fetch the
        // authoritative globe history instead of relying on a stale snapshot.
        refresh();
    }, [challengeRevision, refresh]);
    const onGrabberPointerDown = (event: ReactPointerEvent<HTMLButtonElement>) => {
        dragStartY.current = event.clientY;
        suppressGrabberClick.current = false;
        event.currentTarget.setPointerCapture?.(event.pointerId);
    };
    const onGrabberPointerMove = (event: ReactPointerEvent<HTMLButtonElement>) => {
        if (dragStartY.current === null) return;
        if (Math.abs(event.clientY - dragStartY.current) > 12) suppressGrabberClick.current = true;
    };
    const onGrabberPointerUp = (event: ReactPointerEvent<HTMLButtonElement>) => {
        if (dragStartY.current === null) return;
        const delta = event.clientY - dragStartY.current;
        dragStartY.current = null;
        event.currentTarget.releasePointerCapture?.(event.pointerId);
        if (Math.abs(delta) > 36) {
            suppressGrabberClick.current = true;
            setListOpen(delta < 0);
        }
    };
    const onGrabberClick = () => {
        if (suppressGrabberClick.current) {
            suppressGrabberClick.current = false;
            return;
        }
        toggleList();
    };
    useEffect(() => {
        if (selected) {
            selection.current?.focus({ preventScroll: true });
            selection.current?.scrollIntoView({ block: 'nearest' });
        }
    }, [selected]);
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
            onKeyDown={(event) => {
                if (event.key !== 'Tab') return;
                const controls = Array.from(
                    event.currentTarget.querySelectorAll<HTMLElement>(
                        'button, input, select, textarea, a[href], [tabindex]',
                    ),
                ).filter(
                    (element) =>
                        element.tabIndex >= 0 && !element.matches(':disabled') && element.getClientRects().length > 0,
                );
                const first = controls[0];
                const last = controls[controls.length - 1];
                const target = event.shiftKey
                    ? document.activeElement === first && last
                    : document.activeElement === last && first;
                if (target) {
                    event.preventDefault();
                    target.focus();
                }
            }}
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
                <Globe items={items} selectedID={selectedID} onSelect={selectFromGlobe} />
                <section className={`globe-history${listOpen ? ' is-open' : ''}`} aria-label="Group challenges">
                    <button
                        type="button"
                        className="globe-sheet-grabber"
                        aria-label={listOpen ? 'Collapse geochallenge list' : 'Expand geochallenge list'}
                        aria-expanded={listOpen}
                        aria-controls="globe-history-body"
                        onClick={onGrabberClick}
                        onPointerDown={onGrabberPointerDown}
                        onPointerMove={onGrabberPointerMove}
                        onPointerUp={onGrabberPointerUp}
                        onPointerCancel={() => {
                            dragStartY.current = null;
                        }}
                    >
                        <span className="globe-sheet-grabber-line" aria-hidden="true" />
                        <span className="visually-hidden">
                            {listOpen ? 'Swipe down to collapse' : 'Swipe up to expand'}
                        </span>
                    </button>
                    <div className="globe-history-heading">
                        <h3>Geochallenges</h3>
                        <div className="globe-history-actions">
                            <button
                                type="button"
                                className="globe-refresh"
                                aria-label="Refresh geochallenges"
                                title="Refresh geochallenges"
                                onClick={refresh}
                                disabled={loading}
                            >
                                <Icon name="refresh" />
                            </button>
                        </div>
                    </div>
                    <div id="globe-history-body" className="globe-history-body">
                        <p className="globe-summary">
                            {items.length} {items.length === 1 ? 'challenge' : 'challenges'} · {located} on the globe
                        </p>
                        {items.length > located && (
                            <p className="globe-note">
                                Hidden locations stay off the globe until you're allowed to see them.
                            </p>
                        )}
                        {loading && <p role="status">Loading group challenges…</p>}
                        {error && <p role="alert">{error}</p>}
                        {!loading && !error && items.length === 0 && (
                            <p>No geochallenges yet. Send your first one from the camera!</p>
                        )}
                        {selected && (
                            <div
                                ref={selection}
                                className="globe-selection"
                                role="region"
                                aria-label="Selected challenge"
                                tabIndex={-1}
                            >
                                <strong>{selected.username}'s challenge</strong>
                                <p>
                                    {challengeStatusLabel(selected)} ·{' '}
                                    <time dateTime={selected.created_at}>
                                        {new Date(selected.created_at).toLocaleDateString()}
                                    </time>
                                </p>
                                <p>{locationLabel(selected)}</p>
                                <button
                                    type="button"
                                    className="btn btn-primary"
                                    onClick={() => openChallenge(selected)}
                                >
                                    {selected.status === 'available' ? 'Play challenge' : 'View results'}
                                </button>
                            </div>
                        )}
                        {items.length > 0 && (
                            <ChallengeHistory items={items} selectedID={selectedID} onSelect={setSelectedID} />
                        )}
                    </div>
                </section>
            </div>
        </dialog>
    );
}
