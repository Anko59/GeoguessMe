import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import type { Message, User } from '../types';
import {
    clearCachedSession,
    readCachedMessages,
    readSessionHint,
    saveCachedMessages,
    saveSessionHint,
} from './pwaSessionCache';

const user: User = {
    id: 'user-1',
    username: 'alice',
    email: 'alice@example.test',
    avatar: 'avatar.png',
};

const message: Message = {
    id: 'message-1',
    group_id: 'group-1',
    user_id: 'user-2',
    kind: 'text',
    content: 'Hello',
    created_at: '2026-07-26T00:00:00Z',
};

describe('pwaSessionCache', () => {
    beforeEach(() => localStorage.clear());
    afterEach(() => localStorage.clear());

    it('keeps a bounded per-user, per-group message cache and clears it with the session', () => {
        saveSessionHint(user);
        saveCachedMessages(user.id, 'group-1', [message]);
        saveCachedMessages('user-2', 'group-1', [{ ...message, id: 'other-user' }]);

        expect(readSessionHint()).toEqual(user);
        expect(readCachedMessages(user.id, 'group-1')).toEqual([message]);
        expect(readCachedMessages(user.id, 'group-2')).toEqual([]);

        clearCachedSession();

        expect(readSessionHint()).toBeNull();
        expect(readCachedMessages(user.id, 'group-1')).toEqual([]);
        expect(readCachedMessages('user-2', 'group-1')).toHaveLength(1);
    });
});
