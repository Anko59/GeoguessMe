import { getAPIErrorCodeAsync, publicFeedAPI } from '../api';
import type { ChallengeResults, PublicTimedResults } from '../types';
import type { TimedGameAdapter, TimedGameMedia } from './useTimedGame';

/** Feed challenges use the same phase machine as group challenges while
 * translating the feed's public result contract into the map/score view model. */
export const feedTimedGameAdapter: TimedGameAdapter = {
    async loadResults(id, signal) {
        const results = await publicFeedAPI.timedResults(id, signal);
        let media: TimedGameMedia | undefined;
        let mediaAvailable = true;
        let mediaLoadFailed = false;
        try {
            const blob = await publicFeedAPI.media(id, false, signal);
            media = { blob, mediaType: blob.type || undefined };
        } catch (error) {
            if (signal.aborted) throw error;
            if ((await getAPIErrorCodeAsync(error)) === 'media_removed') mediaAvailable = false;
            else mediaLoadFailed = true;
        }
        return { results: normalizeResults(id, results, mediaAvailable, mediaLoadFailed), media };
    },

    async accept(id, signal) {
        const data = await publicFeedAPI.acceptTimed(id, signal);
        return {
            mediaUrl: data.media_url,
            mediaType: data.media_type,
            viewExpiresAt: data.view_expires_at,
            guessExpiresAt: data.guess_expires_at,
            scoreGraceSeconds: data.score_grace_seconds,
            serverTime: data.server_time,
        };
    },

    async loadMedia(id, window, signal) {
        const blob = await publicFeedAPI.timedMedia(id, signal);
        return { blob, mediaType: blob.type || window.mediaType };
    },

    async mediaDelivered(id, signal) {
        const data = await publicFeedAPI.timedMediaDelivered(id, signal);
        return {
            viewExpiresAt: data.view_expires_at,
            guessExpiresAt: data.guess_expires_at,
            scoreGraceSeconds: data.score_grace_seconds,
            serverTime: data.server_time,
        };
    },

    async guess(id, point, signal) {
        return publicFeedAPI.timedGuess(id, point, signal);
    },

    async timeout(id, signal) {
        await publicFeedAPI.timedTimeout(id, signal);
    },
};

function normalizeResults(
    id: string,
    data: PublicTimedResults,
    mediaAvailable: boolean,
    mediaLoadFailed: boolean,
): ChallengeResults {
    return {
        photo_id: id,
        group_id: 'feed',
        actual_lat: data.actual_lat,
        actual_long: data.actual_long,
        guesses: data.guesses.map((guess) => ({
            id: guess.id,
            photo_id: id,
            group_id: 'feed',
            user_id: guess.user_id,
            username: guess.username,
            avatar: guess.avatar,
            ...(guess.lat === undefined || guess.long === undefined ? {} : { lat: guess.lat, long: guess.long }),
            score: guess.score,
            ...(guess.distance === undefined ? {} : { distance: guess.distance }),
            ...(guess.timed_out ? { timed_out: true } : {}),
            elo_delta: 0,
            created_at: guess.created_at,
        })),
        media_available: mediaAvailable,
        ...(mediaLoadFailed ? { mediaLoadFailed: true } : {}),
        media_url: null,
        media_type: 'image/jpeg',
        server_time: data.server_time,
    };
}
