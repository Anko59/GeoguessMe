import { afterEach, beforeEach, vi } from 'vitest';
import { render } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
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
    react: vi.fn(),
    comments: vi.fn(),
    comment: vi.fn(),
    inbox: vi.fn(),
}));

export function getMocks() {
    return mocks;
}

vi.mock('../../../api', () => ({
    publicFeedAPI: mocks,
    groupsAPI: { inbox: mocks.inbox, markRead: vi.fn() },
    getAPIErrorMessage: (error: Error) => error.message,
    getAPIErrorCode: (error: { response?: { data?: { error?: { code?: string } } } }) =>
        error.response?.data?.error?.code,
    getAPIErrorCodeAsync: async (error: { response?: { data?: { error?: { code?: string } } | Blob } }) => {
        const data = error.response?.data;
        if (typeof Blob !== 'undefined' && data instanceof Blob) {
            try {
                return JSON.parse(await data.text()).error?.code as string | undefined;
            } catch {
                return undefined;
            }
        }
        return data && 'error' in data ? data.error?.code : undefined;
    },
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
                        options: {
                            audience: 'public' | 'friends';
                            caption: string;
                            groupIDs: string[];
                            hideLocation: boolean;
                            idempotencyKey: string;
                        },
                    ) => Promise<unknown>
                )(
                    new Blob(['camera'], { type: 'image/jpeg' }),
                    'capture.jpg',
                    { coords: { latitude: 48.8566, longitude: 2.3522 } } as GeolocationPosition,
                    {
                        audience: 'public',
                        caption: '',
                        groupIDs: [],
                        hideLocation: false,
                        idempotencyKey: '11111111-1111-4111-8111-111111111111',
                    },
                )
                    .then(() => (props.onUploadComplete as () => void)())
                    .catch(() => undefined);
            }}
        >
            Take photo
        </button>
    ),
}));

export function post(overrides: Partial<PublicChallenge> = {}): PublicChallenge {
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

export function renderFeed(initialEntry = '/feed') {
    return render(
        <MemoryRouter initialEntries={[initialEntry]}>
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
