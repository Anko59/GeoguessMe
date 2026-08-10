import type { Message, User } from '../types';

const SESSION_KEY = 'geoguessme:pwa-session:v1';
const MESSAGES_KEY_PREFIX = 'geoguessme:pwa-messages:v1:';
const MAX_CACHED_MESSAGES = 200;

function localStore(): Storage | null {
    try {
        return window.localStorage;
    } catch {
        return null;
    }
}

function messageKey(userID: string, groupID: string): string {
    return `${MESSAGES_KEY_PREFIX}${userID}:${groupID}`;
}

function isUser(value: unknown): value is User {
    if (!value || typeof value !== 'object') return false;
    const user = value as Partial<User>;
    // Email is a verified recovery/contact channel: an unverified account has
    // no email key at all, so a cached session must not be rejected for that.
    return (
        typeof user.id === 'string' &&
        typeof user.username === 'string' &&
        (user.email === undefined || user.email === null || typeof user.email === 'string')
    );
}

function isMessage(value: unknown, groupID: string): value is Message {
    if (!value || typeof value !== 'object') return false;
    const message = value as Partial<Message>;
    return (
        message.group_id === groupID &&
        typeof message.id === 'string' &&
        typeof message.user_id === 'string' &&
        typeof message.content === 'string' &&
        typeof message.created_at === 'string' &&
        (message.kind === 'text' || message.kind === 'challenge' || message.kind === 'system')
    );
}

export function readSessionHint(): User | null {
    const store = localStore();
    if (!store) return null;
    try {
        const value = JSON.parse(store.getItem(SESSION_KEY) ?? 'null') as unknown;
        return isUser(value) ? value : null;
    } catch {
        store.removeItem(SESSION_KEY);
        return null;
    }
}

export function saveSessionHint(user: User): void {
    try {
        localStore()?.setItem(SESSION_KEY, JSON.stringify(user));
    } catch {
        // Private browsing or a full quota must not prevent authentication.
    }
}

export function readCachedMessages(userID: string | undefined, groupID: string | undefined): Message[] {
    if (!userID || !groupID) return [];
    const store = localStore();
    if (!store) return [];
    const key = messageKey(userID, groupID);
    try {
        const value = JSON.parse(store.getItem(key) ?? '[]') as unknown;
        return Array.isArray(value) ? value.filter((item): item is Message => isMessage(item, groupID)) : [];
    } catch {
        store.removeItem(key);
        return [];
    }
}

export function saveCachedMessages(userID: string | undefined, groupID: string | undefined, messages: Message[]): void {
    if (!userID || !groupID) return;
    try {
        localStore()?.setItem(messageKey(userID, groupID), JSON.stringify(messages.slice(-MAX_CACHED_MESSAGES)));
    } catch {
        // Caching is an optional startup optimization.
    }
}

export function clearCachedSession(): void {
    const store = localStore();
    if (!store) return;
    const userID = readSessionHint()?.id;
    store.removeItem(SESSION_KEY);
    if (!userID) return;
    for (let index = store.length - 1; index >= 0; index -= 1) {
        const key = store.key(index);
        if (key?.startsWith(`${MESSAGES_KEY_PREFIX}${userID}:`)) store.removeItem(key);
    }
}
