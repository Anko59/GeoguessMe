import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react';
import type { GroupChallenge, Message } from '../../types';
import GroupHeader from '../navigation/GroupHeader';
import Icon from '../ui/Icon';
import Globe from './Globe';
import ChallengeHistory from './ChallengeHistory';
import { challengeStatusLabel, locationLabel } from './challengeLabels';
import { useGroupChallenges } from './useGroupChallenges';
import './GroupGlobe.css';

interface GroupGlobeProps {
    groupID: string;
    groupName: string;
    groupPhotoURL?: string;
    headerActions?: ReactNode;
    /** Changes when a realtime challenge is published for this group. */
    challengeRevision?: string;
    onClose: () => void;
    onChallenge: (message: Message) => void;
}

export default function GroupGlobe({
    groupID,
    groupName,
    groupPhotoURL = '/logo.png',
    headerActions,
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
    const dragPointerID = useRef<number | null>(null);
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
    const isSheetGestureTarget = (target: EventTarget | null) => {
        if (!(target instanceof Element)) return false;
        if (target.closest('.globe-history-body, .globe-refresh')) return false;
        // The non-scrolling sheet chrome is a safe gesture surface. Keep the
        // scrollable body out of this path so browsing challenges never moves
        // the sheet instead.
        return Boolean(target.closest('.globe-history'));
    };
    const onHistoryPointerDown = (event: ReactPointerEvent<HTMLElement>) => {
        if (!isSheetGestureTarget(event.target)) return;
        dragStartY.current = event.clientY;
        dragPointerID.current = event.pointerId;
        suppressGrabberClick.current = false;
        try {
            event.currentTarget.setPointerCapture?.(event.pointerId);
        } catch {
            // Synthetic pointer events and browsers without pointer capture can
            // still complete a swipe from the events delivered to the sheet.
        }
    };
    const onHistoryPointerMove = (event: ReactPointerEvent<HTMLElement>) => {
        if (dragStartY.current === null || dragPointerID.current !== event.pointerId) return;
        if (Math.abs(event.clientY - dragStartY.current) > 12) suppressGrabberClick.current = true;
    };
    const onHistoryPointerUp = (event: ReactPointerEvent<HTMLElement>) => {
        if (dragStartY.current === null || dragPointerID.current !== event.pointerId) return;
        const delta = event.clientY - dragStartY.current;
        dragStartY.current = null;
        dragPointerID.current = null;
        try {
            if (event.currentTarget.hasPointerCapture?.(event.pointerId)) {
                event.currentTarget.releasePointerCapture?.(event.pointerId);
            }
        } catch {
            // Pointer capture is an enhancement; releasing it is not required
            // for the sheet state transition.
        }
        if (Math.abs(delta) > 36) {
            suppressGrabberClick.current = true;
            setListOpen(delta < 0);
        }
    };
    const onHistoryPointerCancel = (event: ReactPointerEvent<HTMLElement>) => {
        if (dragPointerID.current !== event.pointerId) return;
        dragStartY.current = null;
        dragPointerID.current = null;
        suppressGrabberClick.current = false;
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
            <GroupHeader
                groupName={groupName}
                photoURL={groupPhotoURL}
                eyebrow={groupName}
                heading="Your group's world"
                headingID="group-globe-title"
                headingLevel={2}
                onClose={onClose}
                actions={headerActions}
            />
            <div className="globe-layout">
                <Globe items={items} selectedID={selectedID} onSelect={selectFromGlobe} />
                <section
                    className={`globe-history${listOpen ? ' is-open' : ''}`}
                    aria-label="Group challenges"
                    aria-describedby="globe-sheet-hint"
                    onPointerDown={onHistoryPointerDown}
                    onPointerMove={onHistoryPointerMove}
                    onPointerUp={onHistoryPointerUp}
                    onPointerCancel={onHistoryPointerCancel}
                >
                    <button
                        type="button"
                        className="globe-sheet-grabber"
                        aria-label={listOpen ? 'Collapse geochallenge list' : 'Expand geochallenge list'}
                        aria-expanded={listOpen}
                        aria-controls="globe-history-body"
                        onClick={onGrabberClick}
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
                    <p id="globe-sheet-hint" className="visually-hidden">
                        Swipe the handle or heading up and down to {listOpen ? 'collapse' : 'expand'} the challenge
                        list. Scroll the challenge list itself to browse.
                    </p>
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
