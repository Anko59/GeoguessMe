import type { Message } from '../types';

// compareMessages orders messages by the stable tuple (created_at, id), the
// same order the backend paginates by, so the displayed list and the cursor
// snapshot always agree on which message is newest.
export function compareMessages(a: Message, b: Message): number {
    const byTime = a.created_at.localeCompare(b.created_at);
    if (byTime !== 0) return byTime;
    if (a.id < b.id) return -1;
    return a.id > b.id ? 1 : 0;
}

/**
 * mergeMessages deduplicates incoming messages into the current log by id and
 * re-sorts by the canonical order. Re-delivery of the same message over two
 * paths (a live socket event overlapping a catch-up page, or catch-up pages
 * draining after a reconnect) therefore cannot create a duplicate entry.
 *
 * Per-message reconciliation rules (viewer-independent except for reaction
 * selection, which takes the viewer id):
 * - `reactions` is replaced when the incoming message carries the field and
 *   is preserved otherwise, so a partial live update cannot wipe the counts.
 * - A `reaction_update` delta is applied to the incoming reaction list: the
 *   viewer's own selection flips to `delta.active`, every other reaction keeps
 *   its previous `reacted` flag.
 * - `challenge_status` falls back to the previous value when the incoming
 *   message omits it, and `challenge_resolved` is sticky once true.
 */
export function mergeMessages(current: Message[], incoming: Message[], viewerID?: string): Message[] {
    if (incoming.length === 0) return current;
    const byId = new Map<string, Message>();
    for (const message of current) {
        if (message.id) byId.set(message.id, message);
    }
    for (const message of incoming) {
        if (!message.id) continue;
        const previous = byId.get(message.id);
        let reactions = message.reactions === undefined ? previous?.reactions : (message.reactions ?? []);
        if (message.reaction_update && message.reactions !== undefined) {
            const delta = message.reaction_update;
            reactions = (message.reactions ?? []).map((reaction) => ({
                ...reaction,
                reacted:
                    delta.user_id === viewerID && reaction.reaction === delta.reaction
                        ? delta.active
                        : (previous?.reactions?.find((item) => item.reaction === reaction.reaction)?.reacted ?? false),
            }));
        }
        byId.set(message.id, {
            ...previous,
            ...message,
            challenge_status: message.challenge_status ?? previous?.challenge_status,
            challenge_resolved: Boolean(message.challenge_resolved || previous?.challenge_resolved),
            reactions,
        });
    }
    return [...byId.values()].sort(compareMessages);
}

// lastStableCursor snapshots the newest message id of a sorted log. The
// reconnect sequence uses it as the strict-after anchor for catch-up so
// nothing created during a disconnect is ever skipped.
export function lastStableCursor(messages: Message[]): string {
    return messages.length > 0 ? messages[messages.length - 1].id : '';
}

/**
 * pruneBeforeAnchor drops every cached message strictly older than the anchor,
 * which is the oldest message of the first server-fetched page. A local cache
 * makes the first screen immediate, but the first server sync prunes any
 * cached message older than the fetched page so stale session history can
 * never sit below the live tail with a gap. Live messages arriving during the
 * fetch are newer than the page and survive the prune. A null anchor (empty
 * first page) clears the log entirely.
 */
export function pruneBeforeAnchor(messages: Message[], anchor: Message | null): Message[] {
    if (anchor === null) return [];
    return messages.filter((message) => compareMessages(message, anchor) >= 0);
}
