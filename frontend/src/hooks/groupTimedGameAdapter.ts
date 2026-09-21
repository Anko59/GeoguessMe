import api from '../api';
import type { ChallengeAcceptance, ChallengeMediaDelivered, ChallengeResults, GuessResult } from '../types';
import type { TimedGameAdapter, TimedGameMedia, TimedGameWindow } from './useTimedGame';

/** Adapter for the existing group challenge endpoints. Keeping this wire
 * mapping separate lets the shared timed-game hook stay source-agnostic. */
export function groupTimedGameAdapter(): TimedGameAdapter {
    return groupAdapter;
}

const groupAdapter: TimedGameAdapter = {
    async loadResults(id, signal) {
        const response = await api.get<ChallengeResults>(`/challenges/${id}/results`, { signal });
        let media: TimedGameMedia | undefined;
        if (response.data.media_available && response.data.media_url) {
            media = await loadGroupMedia(response.data.media_url, response.data.media_type ?? undefined, signal);
        }
        return { results: response.data, media };
    },

    async accept(id, signal) {
        const response = await api.post<ChallengeAcceptance>(`/challenges/${id}/accept`, undefined, { signal });
        return normalizeGroupWindow(response.data);
    },

    async loadMedia(_id, window, signal) {
        if (!window.mediaUrl) throw new Error('Challenge media is unavailable.');
        return loadGroupMedia(window.mediaUrl, window.mediaType, signal);
    },

    async mediaDelivered(id, signal) {
        const response = await api.post<ChallengeMediaDelivered>(`/challenges/${id}/media-delivered`, undefined, {
            signal,
        });
        return normalizeGroupDelivered(response.data);
    },

    async guess(id, point, signal) {
        // Keep the group endpoint invocation shape unchanged; the group API
        // has historically relied on the shared axios auth interceptor and
        // does not need a request option for this short mutation.
        void signal;
        const response = await api.post<GuessResult>(`/challenges/${id}/guess`, point);
        return response.data;
    },

    async timeout(id, signal) {
        void signal;
        await api.post(`/challenges/${id}/timeout`);
    },
};

function normalizeGroupWindow(data: ChallengeAcceptance): TimedGameWindow {
    return {
        mediaUrl: data.media_url,
        mediaType: data.media_type,
        viewExpiresAt: data.view_expires_at,
        guessExpiresAt: data.guess_expires_at,
        scoreGraceSeconds: data.score_grace_seconds,
        serverTime: data.server_time,
    };
}

function normalizeGroupDelivered(data: ChallengeMediaDelivered): TimedGameWindow {
    return {
        viewExpiresAt: data.view_expires_at,
        guessExpiresAt: data.guess_expires_at,
        scoreGraceSeconds: data.score_grace_seconds,
        serverTime: data.server_time,
    };
}

async function loadGroupMedia(
    url: string,
    mediaType: string | undefined,
    signal: AbortSignal,
): Promise<TimedGameMedia> {
    if (url.startsWith('http://') || url.startsWith('https://')) return { url, mediaType };
    const apiPath = url.startsWith('/api/v1/') ? url.slice('/api/v1'.length) : url;
    const response = await api.get<Blob>(apiPath, { responseType: 'blob', signal });
    return { blob: response.data, mediaType };
}
