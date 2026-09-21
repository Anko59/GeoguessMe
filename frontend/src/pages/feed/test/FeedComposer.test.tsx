import { act, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import FeedComposer from '../FeedComposer';

const mocks = vi.hoisted(() => ({
    publish: vi.fn(),
    cameraProps: null as Record<string, unknown> | null,
}));
type FeedUpload = (
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
) => Promise<unknown>;
vi.mock('../../../api', () => ({
    publicFeedAPI: { publish: mocks.publish },
    getAPIErrorMessage: (error: Error) => error.message,
}));
vi.mock('../../../components/camera/Camera', () => ({
    default: (props: Record<string, unknown>) => {
        mocks.cameraProps = props;
        return <div data-testid="camera-surface" />;
    },
}));

beforeEach(() => {
    vi.resetAllMocks();
    mocks.cameraProps = null;
});

function renderComposer() {
    const onPublished = vi.fn();
    const view = render(<FeedComposer onClose={vi.fn()} onPublished={onPublished} />);
    return { ...view, onPublished };
}

function position(): GeolocationPosition {
    return { coords: { latitude: 48.8566, longitude: 2.3522 } } as GeolocationPosition;
}

describe('Public feed camera composer', () => {
    it('opens the shared camera surface without a preliminary feed form', () => {
        renderComposer();
        expect(screen.getByTestId('camera-surface')).toBeInTheDocument();
        expect(screen.queryByLabelText('Caption')).not.toBeInTheDocument();
        expect(screen.queryByText(/who can see this challenge/i)).not.toBeInTheDocument();
        expect(screen.queryByLabelText('Latitude')).not.toBeInTheDocument();
        expect(screen.queryByLabelText('Longitude')).not.toBeInTheDocument();
        expect(mocks.cameraProps).toMatchObject({ variant: 'feed' });
    });

    it('maps shared camera options to a multi-destination feed publication', async () => {
        mocks.publish.mockResolvedValue({ id: 'post-1' });
        const { onPublished } = renderComposer();
        const upload = mocks.cameraProps?.uploadCaptured as FeedUpload;

        await act(async () => {
            await upload(new Blob(['camera'], { type: 'image/jpeg' }), 'capture.jpg', position(), {
                audience: 'friends',
                caption: 'A mystery from today',
                groupIDs: ['group-1', 'group-2'],
                hideLocation: true,
                idempotencyKey: '11111111-1111-4111-8111-111111111111',
            });
            (mocks.cameraProps?.onUploadComplete as () => void)();
        });

        await waitFor(() => expect(mocks.publish).toHaveBeenCalledTimes(1));
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('caption')).toBe('A mystery from today');
        expect(form.get('audience')).toBe('friends');
        expect(form.getAll('group_id')).toEqual(['group-1', 'group-2']);
        expect(form.get('hide_location')).toBe('true');
        expect(form.get('idempotency_key')).toBe('11111111-1111-4111-8111-111111111111');
        expect(form.get('lat')).toBe('48.8566');
        expect(form.get('long')).toBe('2.3522');
        expect(form.get('photo')).toBeInstanceOf(Blob);
        expect(onPublished).toHaveBeenCalledWith('post-1');
    });

    it('supports a public-only publication with no selected groups', async () => {
        mocks.publish.mockResolvedValue({ id: 'post-2' });
        renderComposer();
        const upload = mocks.cameraProps?.uploadCaptured as FeedUpload;
        await act(async () => {
            await upload(new Blob(['camera'], { type: 'image/jpeg' }), 'capture.jpg', position(), {
                audience: 'public',
                caption: '',
                groupIDs: [],
                hideLocation: false,
                idempotencyKey: '22222222-2222-4222-8222-222222222222',
            });
        });

        await waitFor(() => expect(mocks.publish).toHaveBeenCalledTimes(1));
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('audience')).toBe('public');
        expect(form.getAll('group_id')).toEqual([]);
        expect(form.get('hide_location')).toBe('false');
    });
});
