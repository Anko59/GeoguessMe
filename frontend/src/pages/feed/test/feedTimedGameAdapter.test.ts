import { beforeEach, describe, expect, it, vi } from 'vitest';
import { feedTimedGameAdapter } from '../../../hooks/feedTimedGameAdapter';

const mocks = vi.hoisted(() => ({
    timedResults: vi.fn(),
    timedMedia: vi.fn(),
    acceptTimed: vi.fn(),
    timedMediaDelivered: vi.fn(),
    timedGuess: vi.fn(),
    timedTimeout: vi.fn(),
}));

vi.mock('../../../api', () => ({ publicFeedAPI: mocks }));

const signal = new AbortController().signal;

beforeEach(() => vi.resetAllMocks());

describe('feed timed-game adapter', () => {
    it('normalizes public results for the shared map and score view', async () => {
        mocks.timedResults.mockResolvedValue({
            challenge_id: 'feed-1',
            actual_lat: 48.8,
            actual_long: 2.3,
            guesses: [
                {
                    id: 'guess-1',
                    user_id: 'user-1',
                    username: 'Alice',
                    avatar: 'alice.png',
                    lat: 48.81,
                    long: 2.31,
                    score: 4900,
                    distance: 1200,
                    timed_out: false,
                    created_at: '2026-01-01T00:00:00Z',
                },
                {
                    id: 'guess-2',
                    user_id: 'user-2',
                    username: 'Bob',
                    avatar: 'bob.png',
                    score: 0,
                    timed_out: true,
                    created_at: '2026-01-01T00:00:01Z',
                },
            ],
            server_time: '2026-01-01T00:00:02Z',
        });
        mocks.timedMedia.mockResolvedValue(new Blob(['photo'], { type: 'image/jpeg' }));

        const loaded = await feedTimedGameAdapter.loadResults('feed-1', signal);

        expect(loaded.results.photo_id).toBe('feed-1');
        expect(loaded.results.actual_lat).toBe(48.8);
        expect(loaded.results.guesses[0]).toMatchObject({
            user_id: 'user-1',
            username: 'Alice',
            avatar: 'alice.png',
            elo_delta: 0,
            distance: 1200,
        });
        expect(loaded.results.guesses[1]).toMatchObject({ user_id: 'user-2', score: 0, timed_out: true });
        expect(loaded.results.guesses[1]).not.toHaveProperty('lat');
        expect(loaded.media).toMatchObject({ mediaType: 'image/jpeg' });
        expect(mocks.timedMedia).toHaveBeenCalledWith('feed-1', signal);
    });

    it('forwards the timed lifecycle without falling back to legacy feed endpoints', async () => {
        const window = {
            media_url: '/feed/challenges/feed-1/timed-media',
            media_type: 'image/jpeg',
            view_expires_at: '2026-01-01T00:00:10Z',
            guess_expires_at: '2026-01-01T00:02:10Z',
            score_grace_seconds: 30,
            server_time: '2026-01-01T00:00:00Z',
        };
        mocks.acceptTimed.mockResolvedValue(window);
        mocks.timedMedia.mockResolvedValue(new Blob(['photo'], { type: 'image/jpeg' }));
        mocks.timedMediaDelivered.mockResolvedValue({
            view_expires_at: window.view_expires_at,
            guess_expires_at: window.guess_expires_at,
            score_grace_seconds: window.score_grace_seconds,
            server_time: window.server_time,
        });
        mocks.timedGuess.mockResolvedValue({ score: 4200, duplicate: false, timed_out: false });
        mocks.timedTimeout.mockResolvedValue(undefined);

        const accepted = await feedTimedGameAdapter.accept('feed-1', signal);
        await feedTimedGameAdapter.loadMedia('feed-1', accepted, signal);
        const delivered = await feedTimedGameAdapter.mediaDelivered('feed-1', signal);
        const guessed = await feedTimedGameAdapter.guess('feed-1', { lat: 48.8, long: 2.3 }, signal);
        await feedTimedGameAdapter.timeout('feed-1', signal);

        expect(accepted.viewExpiresAt).toBe(window.view_expires_at);
        expect(delivered.guessExpiresAt).toBe(window.guess_expires_at);
        expect(guessed.score).toBe(4200);
        expect(mocks.acceptTimed).toHaveBeenCalledWith('feed-1', signal);
        expect(mocks.timedMediaDelivered).toHaveBeenCalledWith('feed-1', signal);
        expect(mocks.timedGuess).toHaveBeenCalledWith('feed-1', { lat: 48.8, long: 2.3 }, signal);
        expect(mocks.timedTimeout).toHaveBeenCalledWith('feed-1', signal);
    });

    it('keeps map results usable when media has been removed', async () => {
        mocks.timedResults.mockResolvedValue({
            challenge_id: 'feed-2',
            actual_lat: 1,
            actual_long: 2,
            guesses: [],
            server_time: '2026-01-01T00:00:00Z',
        });
        mocks.timedMedia.mockRejectedValue(new Error('media expired'));

        const loaded = await feedTimedGameAdapter.loadResults('feed-2', signal);

        expect(loaded.results.actual_lat).toBe(1);
        expect(loaded.media).toBeUndefined();
    });
});
