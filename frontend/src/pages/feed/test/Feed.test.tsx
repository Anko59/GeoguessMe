import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PublicChallenge } from '../../../types';
import Feed from '../Feed';

const mocks = vi.hoisted(() => ({
    list: vi.fn(),
    get: vi.fn(),
    publish: vi.fn(),
    remove: vi.fn(),
    media: vi.fn(),
    acceptTimed: vi.fn(),
    timedMedia: vi.fn(),
    timedMediaDelivered: vi.fn(),
    timedGuess: vi.fn(),
    timedTimeout: vi.fn(),
    timedResults: vi.fn(),
    guess: vi.fn(),
    result: vi.fn(),
    results: vi.fn(),
    react: vi.fn(),
    comments: vi.fn(),
    comment: vi.fn(),
    removeComment: vi.fn(),
    inbox: vi.fn(),
}));
vi.mock('../../../api', () => ({
    publicFeedAPI: mocks,
    groupsAPI: { inbox: mocks.inbox, markRead: vi.fn() },
    getAPIErrorMessage: (error: Error) => error.message,
}));
vi.mock('../../../components/map/Map', () => ({
    default: ({ onLocationSelect }: { onLocationSelect: (lat: number, long: number) => void }) => (
        <button type="button" onClick={() => onLocationSelect(48.8, 2.3)}>
            Select map point
        </button>
    ),
}));
vi.mock('../../../components/camera/Camera', () => ({
    default: (props: Record<string, unknown>) => (
        <button
            type="button"
            aria-label="Take photo"
            onClick={() => {
                void (
                    props.uploadCaptured as (
                        blob: Blob,
                        filename: string,
                        position: GeolocationPosition,
                    ) => Promise<unknown>
                )(new Blob(['camera'], { type: 'image/jpeg' }), 'capture.jpg', {
                    coords: { latitude: 48.8566, longitude: 2.3522 },
                } as GeolocationPosition)
                    .then(() => (props.onUploadComplete as () => void)())
                    .catch(() => undefined);
            }}
        >
            Take photo
        </button>
    ),
}));

function post(overrides: Partial<PublicChallenge> = {}): PublicChallenge {
    return {
        id: 'post-1',
        user_id: 'author',
        username: 'Explorer',
        avatar: 'avatar.png',
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
    mocks.inbox.mockResolvedValue([]);
    mocks.media.mockResolvedValue(new Blob(['image'], { type: 'image/jpeg' }));
    mocks.acceptTimed.mockResolvedValue({
        challenge_id: 'post-1',
        media_url: '/api/v1/feed/challenges/post-1/timed-media',
        media_type: 'image/jpeg',
        accepted_at: new Date().toISOString(),
        view_expires_at: new Date(Date.now() + 1000).toISOString(),
        guess_after: new Date(Date.now() + 1000).toISOString(),
        guess_expires_at: new Date(Date.now() + 121000).toISOString(),
        score_grace_seconds: 30,
        server_time: new Date().toISOString(),
    });
    mocks.timedMedia.mockResolvedValue(new Blob(['timed-image'], { type: 'image/jpeg' }));
    mocks.timedMediaDelivered.mockResolvedValue({
        view_expires_at: new Date(Date.now() + 1000).toISOString(),
        guess_after: new Date(Date.now() + 1000).toISOString(),
        guess_expires_at: new Date(Date.now() + 121000).toISOString(),
        score_grace_seconds: 30,
        server_time: new Date().toISOString(),
    });
    mocks.timedGuess.mockImplementation((id: string, point: { lat: number; long: number }, signal: AbortSignal) =>
        mocks.guess(id, point, signal),
    );
    mocks.timedTimeout.mockResolvedValue(undefined);
    mocks.timedResults.mockResolvedValue({
        challenge_id: 'post-1',
        actual_lat: 48.81,
        actual_long: 2.31,
        guesses: [],
        server_time: new Date().toISOString(),
    });
    mocks.comments.mockResolvedValue({ items: [], next_cursor: '' });
    mocks.results.mockResolvedValue([]);
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
        const expired = new Date(Date.now() - 1000).toISOString();
        mocks.acceptTimed.mockResolvedValueOnce({
            challenge_id: 'post-1',
            media_url: '/api/v1/feed/challenges/post-1/timed-media',
            media_type: 'image/jpeg',
            accepted_at: new Date(Date.now() - 2000).toISOString(),
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
        renderFeed();
        expect(await screen.findByAltText('Blurred preview of an unsolved geo challenge')).toHaveClass(
            'feed-photo-blurred',
        );
        await userEvent.click(screen.getByRole('button', { name: 'Play challenge' }));
        const dialog = await screen.findByRole('dialog', { name: 'Challenge guessing' });
        expect(within(dialog).getByRole('button', { name: 'Select a location…' })).toBeDisabled();
        fireEvent.click(within(dialog).getByRole('button', { name: 'Select map point' }));
        fireEvent.click(within(dialog).getByRole('button', { name: 'Submit guess' }));
        expect(await screen.findByText('4,900 points')).toBeInTheDocument();
        expect(screen.getByText('✓ Revealed')).toBeInTheDocument();
        expect(mocks.timedGuess).toHaveBeenCalledWith('post-1', { lat: 48.8, long: 2.3 }, expect.any(AbortSignal));
        fireEvent.click(screen.getByRole('button', { name: 'Close' }));
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
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

    it('loads ranked challenge results for a revealed post', async () => {
        mocks.list.mockResolvedValue({ items: [post({ resolved: true })], next_cursor: '' });
        mocks.timedResults.mockResolvedValue({
            challenge_id: 'post-1',
            actual_lat: 48.81,
            actual_long: 2.31,
            guesses: [
                {
                    id: 'guess-1',
                    user_id: 'other',
                    username: 'Navigator',
                    avatar: 'avatar-a.png',
                    score: 4800,
                    distance: 100,
                    timed_out: false,
                    lat: 48.81,
                    long: 2.31,
                    created_at: new Date().toISOString(),
                },
                {
                    id: 'guess-2',
                    user_id: 'viewer',
                    username: 'Me',
                    avatar: 'avatar-b.png',
                    score: 4200,
                    distance: 300,
                    timed_out: false,
                    lat: 48.8,
                    long: 2.3,
                    created_at: new Date().toISOString(),
                },
            ],
            server_time: new Date().toISOString(),
        });
        /* Legacy result rows are intentionally no longer used by the timed
         * feed flow; keep the old fixture available for unrelated tests. */
        mocks.results.mockResolvedValue([
            {
                rank: 1,
                user_id: 'other',
                username: 'Navigator',
                avatar: 'avatar-a.png',
                score: 4800,
                distance: 100,
                elo_delta: 4,
                is_viewer: false,
            },
            {
                rank: 2,
                user_id: 'viewer',
                username: 'Me',
                avatar: 'avatar-b.png',
                score: 4200,
                distance: 300,
                elo_delta: -4,
                is_viewer: true,
            },
        ]);
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'Open challenge results' }));
        expect(await screen.findByText('Challenge results')).toBeInTheDocument();
        expect(screen.getByText('Navigator')).toBeInTheDocument();
        expect(screen.getByText('4,800 pts')).toBeInTheDocument();
        expect(screen.getByText('4,200 pts')).toBeInTheDocument();
        expect(mocks.timedResults).toHaveBeenCalledWith('post-1', expect.any(AbortSignal));
    });

    it('reopens the saved result without submitting another guess', async () => {
        mocks.list.mockResolvedValue({ items: [post({ resolved: true })], next_cursor: '' });
        mocks.timedResults.mockResolvedValue({
            challenge_id: 'post-1',
            actual_lat: 48.81,
            actual_long: 2.31,
            guesses: [],
            server_time: new Date().toISOString(),
        });
        renderFeed();
        fireEvent.click(await screen.findByRole('button', { name: 'Open challenge results' }));
        expect(await screen.findByText('Challenge results')).toBeInTheDocument();
        expect(mocks.timedGuess).not.toHaveBeenCalled();
    });

    it('persists reactions, reverses them, and preserves state on failure', async () => {
        mocks.react.mockRejectedValueOnce(new Error('Unable to like')).mockResolvedValue(true);
        renderFeed();
        const likeButton = await screen.findByRole('button', { name: 'Like challenge' });
        expect(screen.getByRole('img', { name: 'Explorer' })).toHaveAttribute('src', '/avatars/avatar.png');
        expect(likeButton.querySelector('img')).toHaveAttribute('src', '/reactions/like.png');
        expect(screen.getByRole('button', { name: '0 comments' }).querySelector('img')).toHaveAttribute(
            'src',
            '/chat_bubbl_icon.png',
        );
        expect(screen.getByRole('button', { name: 'Share challenge' }).querySelector('img')).toHaveAttribute(
            'src',
            '/foward_arrow_icon.png',
        );
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
            avatar: 'avatar-viewer.png',
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
        expect(screen.getByRole('img', { name: 'Me' })).toHaveAttribute('src', '/avatars/avatar-viewer.png');
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

    it('publishes a camera photo with the device location without any group destination', async () => {
        mocks.list.mockResolvedValue({ items: [], next_cursor: '' });
        mocks.publish.mockResolvedValue({ id: 'created' });
        mocks.get.mockResolvedValue(post({ id: 'created', is_owner: true }));
        renderFeed();
        await screen.findByText('The world is waiting for your first post');
        fireEvent.click(screen.getByRole('button', { name: '+ Post a challenge' }));
        const dialog = screen.getByRole('dialog');
        fireEvent.change(within(dialog).getByLabelText('Caption'), { target: { value: 'A new mystery' } });
        fireEvent.click(within(dialog).getByRole('button', { name: 'Take photo' }));
        expect(await screen.findByText('Your challenge')).toBeInTheDocument();
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('photo')).toBeInstanceOf(Blob);
        expect(form.get('caption')).toBe('A new mystery');
        expect(form.get('lat')).toBe('48.8566');
        expect(form.get('long')).toBe('2.3522');
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

    it.each(['publish'] as const)(
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
                fireEvent.click(within(dialog).getByRole('button', { name: 'Take photo' }));
            } else {
                fireEvent.click(within(dialog).getByRole('button', { name: 'Select map point' }));
            }
            const submit = within(dialog).getByRole('button', {
                name: operation === 'publish' ? 'Take photo' : 'Guess & reveal',
            });
            if (operation === 'guess') {
                fireEvent.click(submit);
            }
            expect(mocks[operation]).toHaveBeenCalledTimes(1);
            expect(within(dialog).getByRole('button', { name: 'Close dialog' })).toBeDisabled();
            fireEvent(dialog, new Event('cancel', { cancelable: true }));
            expect(dialog).toBeInTheDocument();
            await act(async () => fail(new Error('Save failed')));
            expect(await within(dialog).findByRole('alert')).toHaveTextContent('Save failed');
            if (operation === 'publish') expect(within(dialog).queryByLabelText('Latitude')).not.toBeInTheDocument();
            else expect(within(dialog).getByLabelText('Latitude')).toHaveValue(48.8);
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
