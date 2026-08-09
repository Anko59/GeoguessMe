// Wire types for the GeoGuessMe API.
//
// Wire-level request/response/schema types are generated from the OpenAPI
// contract (docs/openapi.yaml) into ./openapi.generated.ts and re-exported
// here under the names the frontend already uses. The OpenAPI spec is the
// single source of truth for wire shapes; this module never restates them.
//
// Hand-written types in this module are limited to narrow view-model aliases
// (a wire shape plus a client invariant) and client-only types that have no
// OpenAPI schema: leaderboard query-parameter literals and the tolerant API
// error parse shape. Regenerate the contract with `make openapi-generate`; the
// drift gate is `make openapi-check`.

import type { components } from './openapi.generated';

// --- Wire types (generated from docs/openapi.yaml) ---

export type User = components['schemas']['AuthUser'];
export type ProgressionRank = components['schemas']['ProgressionRank'];
export type GlobalRank = components['schemas']['GlobalRank'];
export type Profile = components['schemas']['Profile'];
export type PublicProfile = components['schemas']['PublicProfile'];
export type AuthResponse = components['schemas']['AuthResponse'];
export type Group = components['schemas']['Group'];
export type Member = components['schemas']['Member'];
export type LeaderboardEntry = components['schemas']['LeaderboardEntry'];
export type ChallengeAcceptance = components['schemas']['ChallengeAccepted'];
export type ChallengeMediaDelivered = components['schemas']['ChallengeMediaDelivered'];
export type GuessResult = components['schemas']['GuessResponse'];
export type MessagesPage = components['schemas']['MessagesPage'];
export type PushSubscriptionRequest = components['schemas']['PushSubscriptionRequest'];

// --- Narrow view-model aliases (wire shape plus client invariants) ---

/** A reaction aggregate as rendered by the client. The deprecated `emoji`
 *  compatibility alias is deliberately omitted (the client never reads it;
 *  PR 12 removes it from the contract), and `usernames` is treated as
 *  optional defensively. Everything else comes from the generated wire type. */
export type Reaction = Omit<components['schemas']['MessageReaction'], 'emoji' | 'usernames'> & {
    usernames?: string[];
};

/** WebSocket reaction mutation metadata without the deprecated `emoji` alias. */
export type ReactionUpdate = Omit<components['schemas']['MessageReactionUpdate'], 'emoji'>;

/** A chat message with the reaction fields narrowed to the client's view
 *  model. All other wire fields, including `content` being optional, come
 *  from the generated contract. */
export type Message = Omit<components['schemas']['Message'], 'reactions' | 'reaction_update'> & {
    reactions?: Reaction[];
    reaction_update?: ReactionUpdate;
};

/** A challenge guess as rendered by the client. The wire contract marks
 *  username/avatar optional, but every guess returned on a resolved
 *  challenge includes them and the map and score cards require them. */
export type ChallengeGuess = Omit<components['schemas']['Guess'], 'username' | 'avatar'> & {
    username: string;
    avatar: string;
};

/** Challenge results with the guesses narrowed to the client's render model. */
export type ChallengeResults = Omit<components['schemas']['ChallengeResults'], 'guesses'> & {
    guesses: ChallengeGuess[];
};

// --- Client-only types (not OpenAPI schemas) ---

/** Tolerant client parse shape for API error responses. The OpenAPI contract
 *  documents `{ error: { code, message } }`; the client additionally tolerates
 *  a flat `{ code?, message? }` envelope defensively. */
export interface APIErrorBody {
    error?: { code: string; message: string };
    code?: string;
    message?: string;
}

/** Leaderboard query-parameter literals (documented inline in the OpenAPI
 *  paths, not as reusable schemas). */
export type LeaderboardPeriod = 'week' | 'month' | 'all';
export type LeaderboardMetric = 'total' | 'average' | 'elo';
