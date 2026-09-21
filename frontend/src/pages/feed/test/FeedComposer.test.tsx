import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import FeedComposer from '../FeedComposer';

const mocks = vi.hoisted(() => ({
    publish: vi.fn(),
    inbox: vi.fn(),
    cameraProps: null as Record<string, unknown> | null,
}));
vi.mock('../../../api', () => ({
    publicFeedAPI: { publish: mocks.publish },
    groupsAPI: { inbox: mocks.inbox },
    getAPIErrorMessage: (error: Error) => error.message,
}));
vi.mock('../../../components/camera/Camera', () => ({
    default: (props: Record<string, unknown>) => {
        mocks.cameraProps = props;
        return (
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
                    } as GeolocationPosition).then(() => (props.onUploadComplete as () => void)());
                }}
            >
                Take photo
            </button>
        );
    },
}));

beforeEach(() => {
    vi.resetAllMocks();
    mocks.cameraProps = null;
    mocks.inbox.mockResolvedValue([]);
});

function renderComposer() {
    const onPublished = vi.fn();
    const view = render(<FeedComposer onClose={vi.fn()} onPublished={onPublished} />);
    return { ...view, onPublished };
}

describe('Public feed camera composer', () => {
    it('uses the camera workflow and never exposes file, map, or coordinate inputs', async () => {
        renderComposer();
        await screen.findByRole('button', { name: 'Take photo' });
        expect(screen.queryByLabelText('Challenge photo')).not.toBeInTheDocument();
        expect(screen.queryByLabelText('Latitude')).not.toBeInTheDocument();
        expect(screen.queryByLabelText('Longitude')).not.toBeInTheDocument();
        expect(screen.queryByText(/enter a location manually/i)).not.toBeInTheDocument();
        expect(mocks.cameraProps).toMatchObject({ variant: 'feed' });
    });

    it('publishes caption, audience, groups, and the camera device location', async () => {
        mocks.inbox.mockResolvedValue([
            { id: 'group-1', name: 'Paris explorers', unread_count: 0, latest_message: null },
        ]);
        mocks.publish.mockResolvedValue({ id: 'post-1' });
        const { onPublished } = renderComposer();
        await userEvent.type(screen.getByLabelText('Caption'), 'A mystery from today');
        await userEvent.click(screen.getByLabelText('Friends in my groups'));
        await userEvent.click(await screen.findByLabelText('Paris explorers'));
        await userEvent.click(screen.getByRole('button', { name: 'Take photo' }));

        await waitFor(() => expect(mocks.publish).toHaveBeenCalledTimes(1));
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('caption')).toBe('A mystery from today');
        expect(form.get('audience')).toBe('friends');
        expect(form.getAll('group_id')).toEqual(['group-1']);
        expect(form.get('lat')).toBe('48.8566');
        expect(form.get('long')).toBe('2.3522');
        expect(form.get('photo')).toBeInstanceOf(Blob);
        expect(onPublished).toHaveBeenCalledWith('post-1');
    });

    it('clears selected groups when switching back to public audience', async () => {
        mocks.inbox.mockResolvedValue([
            { id: 'group-1', name: 'Paris explorers', unread_count: 0, latest_message: null },
        ]);
        mocks.publish.mockResolvedValue({ id: 'post-1' });
        renderComposer();
        await userEvent.click(screen.getByLabelText('Friends in my groups'));
        await userEvent.click(await screen.findByLabelText('Paris explorers'));
        await userEvent.click(screen.getByLabelText('Everyone on GeoGuessMe'));
        await userEvent.click(screen.getByRole('button', { name: 'Take photo' }));

        await waitFor(() => expect(mocks.publish).toHaveBeenCalledTimes(1));
        const form = mocks.publish.mock.calls[0][0] as FormData;
        expect(form.get('audience')).toBe('public');
        expect(form.getAll('group_id')).toEqual([]);
    });

    it('reports group loading failures instead of silently dropping the audience control dependency', async () => {
        mocks.inbox.mockRejectedValue(new Error('Groups are temporarily unavailable'));
        renderComposer();
        await userEvent.click(screen.getByLabelText('Friends in my groups'));
        expect(await screen.findByText('Groups are temporarily unavailable')).toBeInTheDocument();
    });
});
