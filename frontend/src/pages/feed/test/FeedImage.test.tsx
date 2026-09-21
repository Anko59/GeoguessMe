import { act, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import FeedImage from '../FeedImage';

const { media } = vi.hoisted(() => ({ media: vi.fn() }));
vi.mock('../../../api', () => ({ publicFeedAPI: { media } }));

beforeEach(() => {
    vi.resetAllMocks();
    vi.stubGlobal('IntersectionObserver', undefined);
    media.mockResolvedValue(new Blob(['image'], { type: 'image/jpeg' }));
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:feed');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
});

afterEach(() => vi.unstubAllGlobals());

describe('FeedImage', () => {
    it('revokes feed media once and ignores delivery after unmount', async () => {
        const { unmount } = render(<FeedImage id="post-1" revealed={false} />);
        await screen.findByRole('img');
        unmount();
        expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);
        let resolve!: (blob: Blob) => void;
        media.mockReturnValue(new Promise<Blob>((done) => (resolve = done)));
        const pending = render(<FeedImage id="post-2" revealed={false} />);
        pending.unmount();
        await act(async () => resolve(new Blob(['late'])));
        expect(URL.createObjectURL).toHaveBeenCalledTimes(1);
    });

    it('loads nearby photos and releases their blobs when they leave the viewport', async () => {
        let visibility!: IntersectionObserverCallback;
        const disconnect = vi.fn();
        class Observer {
            constructor(callback: IntersectionObserverCallback) {
                visibility = callback;
            }
            observe() {}
            disconnect = disconnect;
        }
        vi.stubGlobal('IntersectionObserver', Observer);
        try {
            const view = render(<FeedImage id="post-1" revealed={false} />);
            expect(media).not.toHaveBeenCalled();
            act(() => visibility([{ isIntersecting: true } as IntersectionObserverEntry], {} as IntersectionObserver));
            await screen.findByRole('img');
            act(() => visibility([{ isIntersecting: false } as IntersectionObserverEntry], {} as IntersectionObserver));
            expect(screen.queryByRole('img')).not.toBeInTheDocument();
            expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);
            view.unmount();
            expect(disconnect).toHaveBeenCalledTimes(1);
        } finally {
            vi.unstubAllGlobals();
        }
    });
});
