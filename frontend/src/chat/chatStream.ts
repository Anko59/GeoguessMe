import type { Message } from '../types';
import { PAGE_SIZE, type ChatConnectionStatus } from './chatSocketController';
import { mergeMessages, pruneBeforeAnchor } from './messageLog';

// The chat stream state machine. Every action is bound to the cache session
// (user:group) it was created for; a session switch resets the whole stream
// through a 'reset' transition, and any action carrying a superseded session's
// identity is dropped instead of applied. That single guard is what keeps a
// stale socket event, abandoned catch-up page, or delayed older-page response
// from ever reaching a newer group or user's view.

export interface ChatStreamState {
    /** The cache session identity (userID:groupId) this stream belongs to. */
    identity: string;
    /** The viewer id, used to reconcile reaction selection on merges. */
    viewerID?: string;
    messages: Message[];
    connectionStatus: ChatConnectionStatus;
    error: string;
    loadingOlder: boolean;
    hasMoreOlder: boolean;
}

export type ChatStreamAction =
    | { type: 'reset'; identity: string; viewerID?: string; cached: Message[] }
    | { type: 'status'; identity: string; status: ChatConnectionStatus }
    | { type: 'error'; identity: string; message: string }
    | { type: 'merge'; identity: string; incoming: Message[] }
    | { type: 'first_page'; identity: string; items: Message[] }
    | {
          type: 'challenge_status';
          identity: string;
          photoId: string;
          status: NonNullable<Message['challenge_status']>;
      }
    | { type: 'load_older_start'; identity: string }
    | { type: 'load_older_done'; identity: string; items: Message[] }
    | { type: 'load_older_error'; identity: string; message: string };

export function initialChatStreamState(
    identity: string,
    viewerID: string | undefined,
    cached: Message[],
): ChatStreamState {
    return {
        identity,
        viewerID,
        messages: cached,
        connectionStatus: 'connecting',
        error: '',
        loadingOlder: false,
        hasMoreOlder: false,
    };
}

export function chatStreamReducer(state: ChatStreamState, action: ChatStreamAction): ChatStreamState {
    if (action.type === 'reset') {
        // A reset for the already-active session is a no-op (e.g. the mount
        // effect re-dispatching the initial identity).
        if (action.identity === state.identity) return state;
        return initialChatStreamState(action.identity, action.viewerID, action.cached);
    }
    // Stale action from a superseded session: never apply it.
    if (action.identity !== state.identity) return state;
    switch (action.type) {
        case 'status':
            return { ...state, connectionStatus: action.status };
        case 'error':
            return { ...state, error: action.message };
        case 'merge':
            return { ...state, messages: mergeMessages(state.messages, action.incoming, state.viewerID) };
        case 'first_page':
            return {
                ...state,
                messages: pruneBeforeAnchor(state.messages, action.items.length > 0 ? action.items[0] : null),
                hasMoreOlder: action.items.length >= PAGE_SIZE,
            };
        case 'challenge_status':
            return {
                ...state,
                messages: state.messages.map((message) =>
                    message.kind === 'challenge' && message.photo_id === action.photoId
                        ? { ...message, challenge_status: action.status }
                        : message,
                ),
            };
        case 'load_older_start':
            return { ...state, loadingOlder: true };
        case 'load_older_done':
            return {
                ...state,
                messages: mergeMessages(state.messages, action.items, state.viewerID),
                hasMoreOlder: action.items.length >= PAGE_SIZE,
                loadingOlder: false,
            };
        case 'load_older_error':
            return { ...state, error: action.message, loadingOlder: false };
    }
}
