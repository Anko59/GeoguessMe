import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicChallenge } from '../../../types';
import Feed from '../Feed';
import FeedImage from '../FeedImage';

const mocks = vi.hoisted(() => ({
    list: vi.fn(),
    get: vi.fn(),
    publish: vi.fn(),
    remove: vi.fn(),
    media: vi.fn(),
    guess: vi.fn(),
    result: vi.fn(),
    react: vi.fn(),
    comments: vi.fn(),
    comment: vi.fn(),
    removeComment: vi.fn(),
}));
vi.mock('../../../api', () => ({ publicFeedAPI: mocks, getAPIErrorMessage: (error: Error) => error.message }));
vi.mock('../../../components/map/Map', () => ({
    default: ({ onLocationSelect }: { onLocationSelect: (lat: number, long: number) => void }) => (
        <button type="button" onClick={() => onLocationSelect(48.8, 2.3)}>
            Select map point
        </button>
    ),
}));

function post(overrides: Partial<PublicChallenge> = {}): PublicChallenge {
    return {
        id: 'post-1',
        user_id: 'author',
        username: 'Explorer',
        caption: 'A little corner of the world',
        created_at: '2026-09-12T10:00:00Z',
        is_owner: false,
        resolved: false,
        reacted: false,
        reaction_count: 2,
        comment_count: 0,
        ...overrides,
    };
}

function renderFeed() {
    return render(
        <MemoryRouter initialEntries={['/feed']}>
            <Routes>
                <Route path="/feed" element={<Feed />} />
                <Route path="/feed/:id" element={<Feed />} />
            </Routes>
        </MemoryRouter>,
    );
}

beforeEach(() => {
    vi.resetAllMocks();
    vi.stubGlobal('IntersectionObserver', undefined);
    mocks.list.mockResolvedValue({ items: [post()], next_cursor: '' });
    mocks.media.mockResolvedValue(new Blob(['image'], { type: 'image/jpeg' }));
    mocks.comments.mockResolvedValue({ items: [], next_cursor: '' });
    mocks.react.mockResolvedValue(true);
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:feed');
    vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {});
    vi.stubGlobal(
        'createImageBitmap',
        vi.fn().mockImplementation(async () => ({ width: 800, height: 600, close: vi.fn() })),
    );
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
        drawImage: vi.fn(),
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLDialogElement.prototype, 'showModal').mockImplementation(function (this: HTMLDialogElement) {
        this.open = true;
    });
    vi.spyOn(HTMLDialogElement.prototype, 'close').mockImplementation(function (this: HTMLDialogElement) {
        this.open = false;
    });
});

afterEach(() => vi.unstubAllGlobals());

describe('Public feed', () => {
    it('blurs unsolved photos and reveals only after a successful guess', async () => {
        mocks.guess.mockRejectedValueOnce(new Error('Connection lost')).mockResolvedValueOnce({
            score: 4900,
            distance: 150,
            lat: 48.8,
            long: 2.3,
            actual_lat: 48.81,
            actual_long: 2.31,
        });
        renderFeed();
        expect(await screen.findByAltText('Blurred preview of an unsolved geo challenge')).toHaveClass(
            'feed-photo-blurred',
        );
        await userEvent.click(screen.getByRole('button', { name: 'Play challenge' }));
        const dialog = screen.getByRole('dialog', { name: 'Where in the world?' });
        expect(within(dialog).getByRole('button', { name: 'Guess & reveal' })).toBeDisabled();
        fireEvent.click(within(dialog).getByRole('button', { name: 'Select map point' }));
        fireEvent.click(within(dialog).getByRole('button', { name: 'Guess & reveal' }));
        expect(await within(dialog).findByRole('alert')).toHaveTextContent('Connection lost');
        expect(screen.getByAltText('Blurred preview of an unsolved geo challenge')).toBeInTheDocument();
        fireEvent.click(within(dialog).getByRole('button', { name: 'Guess & reveal' }));
        expect(await screen.findByText('4,900 points')).toBeInTheDocument();
        expect(screen.getByText('✓ Revealed')).toBeInTheDocument();
        await waitFor(() =>
            expect(screen.queryByAltText('Blurred preview of an unsolved geo challenge')).not.toBeInTheDocument(),
        );
        expect(mocks.guess).toHaveBeenCalledWith('post-1', { lat: 48.8, long: 2.3 }, expect.any(AbortSignal));
        fireEvent.click(screen.getByRole('button', { name: 'Back to the feed' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'View your result' })).toHaveFocus();
    });

    it('shows owners and previous guessers clear photos', async () => {
        mocks.list.mockResolvedValue({
            items: [post({ is_owner: true }), post({ id: 'post-2', resolved: true })],
            next_cursor: '',
        });
        renderFeed();
        await waitFor(() => expect(screen.getAllByAltText('Geo challenge photo')).toHaveLength(2));
        expect(screen.queryByRole('button', { name: 'Play challenge' })).not.toBeInTheDocument();
        expect(screen.getByText('Your challenge')).toBeInTheDocument();
    });

    it('reopens the saved result without submitting another guess', async () => {
        mocks.list.mockResolvedValue({ items: [post({ resolved: true })], next_cursor: '' });
        mocks.result.mockResolvedValue({
            score: 4200,
            distance: 300,
            lat: 48.8,
            long: 2.3,
            actual_lat: 48.81,
            actual_long: 2.31,
        });
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'View your result' }));
        expect(await screen.findByText('4,200 points')).toBeInTheDocument();
        expect(mocks.guess).not.toHaveBeenCalled();
    });

    it('persists reactions, reverses them, and preserves state on failure', async () => {
        mocks.react.mockRejectedValueOnce(new Error('Unable to like')).mockResolvedValue(true);
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'Like challenge' }));
        expect(await screen.findByRole('alert')).toHaveTextContent('Unable to like');
        expect(screen.getByText('2 likes')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Like challenge' }));
        expect(await screen.findByRole('button', { name: 'Unlike challenge' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByText('3 likes')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Unlike challenge' }));
        await waitFor(() => expect(screen.getByText('2 likes')).toBeInTheDocument());
        expect(mocks.react).toHaveBeenLastCalledWith('post-1', false, expect.any(AbortSignal));
    });

    it('opens comments deliberately and supports posting and moderation', async () => {
        const comment = {
            id: 'comment-1',
            user_id: 'viewer',
            username: 'Me',
            content: 'Beautiful place!',
            created_at: '2026-09-12T11:00:00Z',
            can_delete: true,
        };
        mocks.comment.mockResolvedValue(comment);
        mocks.removeComment.mockResolvedValue(true);
        renderFeed();
        expect(await screen.findByText('Comments may contain clues or spoilers.')).toBeInTheDocument();
        expect(mocks.comments).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: '0 comments' }));
        await screen.findByText('No comments yet. Start the conversation.');
        fireEvent.change(screen.getByRole('textbox', { name: 'Add a comment' }), {
            target: { value: ' Beautiful place! ' },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Post comment' }));
        expect(await screen.findByText('Beautiful place!')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '1 comment' })).toBeInTheDocument();
        expect(mocks.comment).toHaveBeenCalledWith('post-1', 'Beautiful place!', expect.any(AbortSignal));
        fireEvent.click(screen.getByRole('button', { name: 'Delete comment by Me' }));
        await waitFor(() => expect(screen.queryByText('Beautiful place!')).not.toBeInTheDocument());
        expect(screen.getByRole('button', { name: '0 comments' })).toBeInTheDocument();
    });

    it('keeps a new comment count when an earlier reaction request finishes later', async () => {
        let finishLike!: (value: boolean) => void;
        mocks.react.mockReturnValue(
            new Promise<boolean>((resolve) => {
                finishLike = resolve;
            }),
        );
        mocks.comment.mockResolvedValue({
            id: 'comment-1',
            user_id: 'viewer',
            username: 'Me',
            content: 'Lovely!',
            created_at: '2026-09-12T11:00:00Z',
            can_delete: true,
        });
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'Like challenge' }));
        fireEvent.click(screen.getByRole('button', { name: '0 comments' }));
        await screen.findByText('No comments yet. Start the conversation.');
        fireEvent.change(screen.getByRole('textbox', { name: 'Add a comment' }), { target: { value: 'Lovely!' } });
        fireEvent.click(screen.getByRole('button', { name: 'Post comment' }));
        await screen.findByRole('button', { name: '1 comment' });
        await act(async () => finishLike(true));
        expect(screen.getByRole('button', { name: '1 comment' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Unlike challenge' })).toBeInTheDocument();
    });

    it('waits for the initial comment snapshot before accepting a new comment', async () => {
        let finishLoad!: (page: { items: []; next_cursor: string }) => void;
        mocks.comments.mockReturnValue(
            new Promise((resolve) => {
                finishLoad = resolve;
            }),
        );
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: '0 comments' }));
        await screen.findByText('Loading comments…');
        expect(screen.getByRole('textbox', { name: 'Add a comment' })).toBeDisabled();
        await act(async () => finishLoad({ items: [], next_cursor: '' }));
        expect(screen.getByRole('textbox', { name: 'Add a comment' })).toBeEnabled();
    });

    it('loads another page without duplicating cards and retries a failed load', async () => {
        mocks.list
            .mockRejectedValueOnce(new Error('Unable to load feed'))
            .mockResolvedValueOnce({ items: [post()], next_cursor: 'next' })
            .mockResolvedValueOnce({ items: [post(), post({ id: 'post-2', username: 'Traveler' })], next_cursor: '' });
        renderFeed();
        await screen.findByRole('alert');
        fireEvent.click(screen.getByRole('button', { name: 'Try again' }));
        fireEvent.click(await screen.findByRole('button', { name: 'More adventures' }));
        await screen.findByRole('article', { name: 'Geo challenge by Traveler' });
        expect(screen.getAllByRole('article')).toHaveLength(2);
        expect(mocks.list).toHaveBeenLastCalledWith('next', expect.any(AbortSignal));
    });

    it('publishes a photo and explicit location without any group destination', async () => {
        mocks.list.mockResolvedValue({ items: [], next_cursor: '' });
        mocks.publish.mockResolvedValue({ id: 'created' });
        mocks.get.mockResolvedValue(post({ id: 'created', is_owner: true }));
        renderFeed();
        await screen.findByText('The world is waiting for your first post');
        fireEvent.click(screen.getByRole('button', { name: '+ Post a challenge' }));
        const dialog = screen.getByRole('dialog');
        const file = new File(['photo'], 'place.jpg', { type: 'image/jpeg' });
        await userEvent.upload(within(dialog).getByLabelText('Challenge photo'), file);
        fireEvent.change(within(dialog).getByLabelText('Caption'), { target: { value: 'A new mystery' } });
        fireEvent.change(within(dialog).getByLabelText('Latitude'), { target: { value: '48.8' } });
        expect(within(dialog).getByRole('button', { name: 'Publish challenge' })).toBeDisabled();
        fireEvent.change(within(dialog).getByLabelText('Longitude'), { target: { value: '2.3' } });
        expect(within(dialog).getByRole('button', { name: 'Publish challenge' })).toBeEnabled();
        fireEvent.change(within(dialog).getByLabelText('Longitude'), { target: { value: '181' } });
        expect(within(dialog).getByRole('button', { name: 'Publish challenge' })).toBeDisabled();
        fireEvent.change(within(dialog).getByLabelText('Longitude'), { target: { value: '2.3' } });
        fireEvent.click(within(dialog).getByRole('button', { name: 'Publish challenge' }));
        expect(await screen.findByText('Your challenge')).toBeInTheDocument();
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('photo')).toBe(file);
        expect(form.get('caption')).toBe('A new mystery');
        expect(form.get('lat')).toBe('48.8');
        expect(form.has('group_ids')).toBe(false);
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('requires confirmation before removing an authored post', async () => {
        mocks.list.mockResolvedValue({ items: [post({ is_owner: true })], next_cursor: '' });
        mocks.remove.mockResolvedValue(true);
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'Delete post' }));
        expect(mocks.remove).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'Confirm delete' }));
        await waitFor(() => expect(screen.queryByRole('article')).not.toBeInTheDocument());
        expect(mocks.remove).toHaveBeenCalledWith('post-1', expect.any(AbortSignal));
    });

    it.each(['publish', 'guess'] as const)(
        'keeps the %s dialog open during a save and allows recovery after failure',
        async (operation) => {
            let fail!: (error: Error) => void;
            mocks[operation].mockReturnValue(
                new Promise((_, reject) => {
                    fail = reject;
                }),
            );
            renderFeed();
            await screen.findByRole('article');
            fireEvent.click(
                screen.getByRole('button', { name: operation === 'publish' ? '+ Post a challenge' : 'Play challenge' }),
            );
            const dialog = screen.getByRole('dialog');
            if (operation === 'publish') {
                await userEvent.upload(
                    within(dialog).getByLabelText('Challenge photo'),
                    new File(['photo'], 'place.jpg', { type: 'image/jpeg' }),
                );
            }
            fireEvent.click(within(dialog).getByRole('button', { name: 'Select map point' }));
            const submit = within(dialog).getByRole('button', {
                name: operation === 'publish' ? 'Publish challenge' : 'Guess & reveal',
            });
            fireEvent.click(submit);
            fireEvent.click(submit);
            expect(mocks[operation]).toHaveBeenCalledTimes(1);
            expect(within(dialog).getByRole('button', { name: 'Close dialog' })).toBeDisabled();
            fireEvent(dialog, new Event('cancel', { cancelable: true }));
            expect(dialog).toBeInTheDocument();
            await act(async () => fail(new Error('Save failed')));
            expect(await within(dialog).findByRole('alert')).toHaveTextContent('Save failed');
            expect(within(dialog).getByLabelText('Latitude')).toHaveValue(48.8);
            expect(within(dialog).getByRole('button', { name: 'Close dialog' })).toBeEnabled();
            fireEvent.click(within(dialog).getByRole('button', { name: 'Close dialog' }));
            expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
        },
    );

    it('cancels the feed request and ignores a late response after leaving the page', async () => {
        let finish!: (value: { items: PublicChallenge[]; next_cursor: string }) => void;
        mocks.list.mockReturnValue(
            new Promise((resolve) => {
                finish = resolve;
            }),
        );
        const view = renderFeed();
        await waitFor(() => expect(mocks.list).toHaveBeenCalledTimes(1));
        const signal = mocks.list.mock.calls[0][1] as AbortSignal;
        view.unmount();
        expect(signal.aborted).toBe(true);
        await act(async () => finish({ items: [post()], next_cursor: '' }));
        expect(mocks.media).not.toHaveBeenCalled();
    });
});

it('revokes feed media once and ignores delivery after unmount', async () => {
    const { unmount } = render(<FeedImage id="post-1" revealed={false} />);
    await screen.findByRole('img');
    unmount();
    expect(URL.revokeObjectURL).toHaveBeenCalledTimes(1);
    let resolve!: (blob: Blob) => void;
    mocks.media.mockReturnValue(
        new Promise<Blob>((done) => {
            resolve = done;
        }),
    );
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
        expect(mocks.media).not.toHaveBeenCalled();
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
