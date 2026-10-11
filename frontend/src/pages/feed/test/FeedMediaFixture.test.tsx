import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { getMediaFixture, settleFeedMedia } from './FeedTestHarness';
import FeedImage from '../FeedImage';
import { createFeedMediaFixture } from './FeedMediaFixture';

describe('Feed media fixture ownership', () => {
    it('drains requests created by earlier promise deliveries instead of leaving a later wave pending', async () => {
        const fixture = createFeedMediaFixture();
        const chain = fixture
            .defer('public')
            .then(() => fixture.defer('timed'))
            .then(() => fixture.defer('public'));
        expect(fixture.requests).toHaveLength(1);
        expect(fixture.pendingCount).toBe(1);
        await fixture.settle();
        await chain;
        expect(fixture.requests).toHaveLength(3);
        expect(fixture.requests.every((request) => request.settled)).toBe(true);
        expect(fixture.pendingCount).toBe(0);
    });

    it('keeps real images pending until async act settlement and owns every reveal-key replacement', async () => {
        const view = render(<FeedImage id="post-1" revealed={false} />);
        const fixture = getMediaFixture();
        expect(screen.getByText('Loading photo…')).toBeInTheDocument();
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
        expect(fixture.pendingCount).toBe(1);
        expect(URL.createObjectURL).not.toHaveBeenCalled();
        await settleFeedMedia();
        expect(screen.getByAltText('Blurred preview of an unsolved geo challenge')).toBeInTheDocument();

        view.rerender(<FeedImage id="post-1" revealed />);
        expect(fixture.requests[0].signal?.aborted).toBe(true);
        expect(fixture.pendingCount).toBe(1);
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
        expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);
        await settleFeedMedia();
        expect(screen.getByAltText('Geo challenge photo')).toBeInTheDocument();
        expect(fixture.requests.every((request) => request.settled)).toBe(true);
        expect(fixture.pendingCount).toBe(0);
        expect(URL.createObjectURL).toHaveBeenCalledTimes(2);
        view.unmount();
        expect(URL.revokeObjectURL).toHaveBeenCalledTimes(2);
    });

    it('delivers deliberately late blobs only after real cancellation without URLs or late state', async () => {
        const view = render(
            <>
                <FeedImage id="post-1" revealed={false} />
                <FeedImage id="post-2" revealed />
            </>,
        );
        const fixture = getMediaFixture();
        expect(fixture.pendingCount).toBe(2);
        expect(fixture.requests.every((request) => request.signal?.aborted === false)).toBe(true);
        view.unmount();
        expect(fixture.requests.every((request) => request.signal?.aborted === true)).toBe(true);
        await settleFeedMedia();
        expect(fixture.pendingCount).toBe(0);
        expect(fixture.requests.every((request) => request.settled)).toBe(true);
        expect(URL.createObjectURL).not.toHaveBeenCalled();
        expect(URL.revokeObjectURL).not.toHaveBeenCalled();
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
        expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    });
});
