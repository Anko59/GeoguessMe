import { act, screen, waitFor, within } from '@testing-library/react';
import type { PublicChallenge } from '../../../types';
import { describe, expect, it } from 'vitest';
import { clickFeed, getMediaFixture, getMocks, post, renderFeed, settleFeedMedia } from './FeedTestHarness';

const mocks = getMocks();

describe('Public feed comment counts', () => {
    it.each(['/feed', '/feed/post-1'])(
        'refreshes the public comment count after guessing from %s while a new comment arrives',
        async (route) => {
            const initial = post({ comment_count: 3 });
            const updated = post({ comment_count: 4, resolved: true });
            mocks.list.mockResolvedValue({ items: [initial], next_cursor: '' });
            let finishRefresh!: (value: PublicChallenge) => void;
            const refreshed = new Promise<PublicChallenge>((resolve) => {
                finishRefresh = resolve;
            });
            if (route === '/feed/post-1') mocks.get.mockResolvedValueOnce(initial);
            mocks.get.mockReturnValueOnce(refreshed);
            mocks.comments.mockResolvedValue({
                items: [
                    {
                        id: 'new-comment',
                        user_id: 'author',
                        username: 'Explorer',
                        content: 'New comment',
                        created_at: initial.created_at,
                    },
                ],
                next_cursor: 'older',
            });
            const expired = new Date(Date.now() - 1000).toISOString();
            mocks.acceptTimed.mockResolvedValueOnce({
                challenge_id: 'post-1',
                media_url: '/api/v1/feed/challenges/post-1/timed-media',
                media_type: 'image/jpeg',
                accepted_at: expired,
                view_expires_at: expired,
                guess_after: expired,
                guess_expires_at: new Date(Date.now() + 120000).toISOString(),
                score_grace_seconds: 30,
                server_time: new Date().toISOString(),
            });
            mocks.timedMediaDelivered.mockResolvedValueOnce({
                view_expires_at: expired,
                guess_after: expired,
                guess_expires_at: new Date(Date.now() + 120000).toISOString(),
                score_grace_seconds: 30,
                server_time: new Date().toISOString(),
            });
            mocks.timedGuess.mockResolvedValueOnce({
                score: 4900,
                distance: 150,
                timed_out: false,
                duplicate: false,
                guess_id: 'guess-1',
                challenge_id: 'post-1',
                created_at: new Date().toISOString(),
                server_time: new Date().toISOString(),
            });
            await renderFeed(route);
            if (route === '/feed') {
                await screen.findByAltText('Blurred preview of an unsolved geo challenge');
                await clickFeed(screen.getByRole('button', { name: 'Play challenge' }));
            }
            const dialog = await screen.findByRole('dialog', { name: 'Challenge guessing' });
            await clickFeed(within(dialog).getByRole('button', { name: 'Select map point' }));
            await clickFeed(within(dialog).getByRole('button', { name: 'Submit guess' }));
            await screen.findByText('4,900 points');
            const results = await screen.findByRole('dialog', { name: 'Challenge results' });
            await within(results).findByText('New comment');
            await within(results).findByAltText('Challenge location');
            if (route === '/feed') await screen.findByAltText('Geo challenge photo');
            // Blob settlement must not accidentally resolve the post snapshot.
            // A new comment is already visible while the refresh is still pending.
            expect(within(results).getByText('3 comments')).toBeInTheDocument();
            await act(async () => finishRefresh(updated));
            await settleFeedMedia();
            await waitFor(() => expect(within(results).getByText('4 comments')).toBeInTheDocument());
            expect(mocks.get).toHaveBeenCalledTimes(route === '/feed' ? 1 : 2);
            const media = getMediaFixture();
            expect(media.requests.some((request) => request.kind === 'timed')).toBe(true);
            expect(media.requests.every((request) => request.settled)).toBe(true);
            expect(media.pendingCount).toBe(0);
        },
    );
});
