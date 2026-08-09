import { useMemo } from 'react';
import type { Message } from '../../../types';

/** Build an ID → message lookup so reply targeting is O(1) per message. */
export function buildMessageIndex(messages: Message[]): ReadonlyMap<string, Message> {
    const index = new Map<string, Message>();
    for (const message of messages) index.set(message.id, message);
    return index;
}

/**
 * Memoized ID → message index for a message list. The index is rebuilt only
 * when the list identity changes, so the render loop never scans the list to
 * resolve a reply target (the previous `messages.find(...)` per row was an
 * O(n²) reply lookup).
 */
export function useMessageIndex(messages: Message[]): ReadonlyMap<string, Message> {
    return useMemo(() => buildMessageIndex(messages), [messages]);
}
