import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import FeedComposer from '../FeedComposer';

const mocks = vi.hoisted(() => ({ publish: vi.fn() }));
vi.mock('../../../api', () => ({ publicFeedAPI: mocks, getAPIErrorMessage: (error: Error) => error.message }));
vi.mock('../../../components/map/Map', () => ({ default: () => null }));

function bitmap(width = 800, height = 600) {
    return { width, height, close: vi.fn() } as unknown as ImageBitmap;
}

function deferredBitmap() {
    let resolve!: (value: ImageBitmap) => void;
    let reject!: (reason: Error) => void;
    const promise = new Promise<ImageBitmap>((done, fail) => {
        resolve = done;
        reject = fail;
    });
    return { promise, resolve, reject };
}

function renderComposer() {
    const view = render(<FeedComposer onClose={vi.fn()} onPublished={vi.fn()} />);
    fireEvent.change(screen.getByLabelText('Latitude'), { target: { value: '48.8' } });
    fireEvent.change(screen.getByLabelText('Longitude'), { target: { value: '2.3' } });
    return {
        ...view,
        input: screen.getByLabelText('Challenge photo'),
        submit: screen.getByRole('button', { name: 'Publish challenge' }),
    };
}

beforeEach(() => {
    vi.resetAllMocks();
    vi.stubGlobal(
        'createImageBitmap',
        vi.fn().mockImplementation(async () => bitmap()),
    );
    vi.spyOn(URL, 'createObjectURL');
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

describe('Public photo preview', () => {
    it.each(['image/jpeg', 'image/png', 'image/webp'])(
        'renders %s as bounded pixels without exposing a raw file URL',
        async (type) => {
            const image = bitmap();
            vi.mocked(createImageBitmap).mockResolvedValue(image);
            const { input, submit, container } = renderComposer();
            const file = new File(['photo'], '<img src=x onerror=alert(1)>.jpg', { type });
            await userEvent.upload(input, file);
            const preview = await screen.findByRole('img', { name: 'Photo to publish' });
            expect(preview.tagName).toBe('CANVAS');
            expect(preview).toHaveAttribute('width', '320');
            expect(preview).toHaveAttribute('height', '240');
            expect(preview).not.toHaveAttribute('src');
            expect(container.querySelector('img, object, iframe, embed')).toBeNull();
            expect(createImageBitmap).toHaveBeenCalledWith(file);
            expect(image.close).toHaveBeenCalledTimes(1);
            expect(URL.createObjectURL).not.toHaveBeenCalled();
            expect(submit).toBeEnabled();
        },
    );

    it.each(['text/html', 'image/svg+xml', 'application/octet-stream', ''])(
        'rejects unsupported content type %j even when the picker filter is bypassed',
        async (type) => {
            const { input, submit } = renderComposer();
            const file = new File(['<svg onload="alert(1)"></svg>'], 'place.jpg', { type });
            await userEvent.setup({ applyAccept: false }).upload(input, file);
            expect(await screen.findByRole('alert')).toHaveTextContent('Choose a valid JPG, PNG, or WebP photo.');
            expect(input).toHaveAttribute('aria-invalid', 'true');
            expect(input).toHaveAccessibleDescription('Choose a valid JPG, PNG, or WebP photo.');
            expect(submit).toBeDisabled();
            expect(screen.queryByRole('img')).not.toBeInTheDocument();
            expect(createImageBitmap).not.toHaveBeenCalled();
            expect(URL.createObjectURL).not.toHaveBeenCalled();
        },
    );

    it('blocks publishing during decoding and after invalid bytes, then recovers without losing input', async () => {
        const decode = deferredBitmap();
        vi.mocked(createImageBitmap).mockReturnValueOnce(decode.promise);
        const { input, submit } = renderComposer();
        fireEvent.change(screen.getByLabelText('Caption'), { target: { value: 'A mystery' } });
        await userEvent.upload(input, new File(['<script>alert(1)</script>'], 'place.jpg', { type: 'image/jpeg' }));
        expect(screen.getByRole('status')).toHaveTextContent('Preparing photo');
        expect(submit).toBeDisabled();
        fireEvent.submit(submit.closest('form')!);
        expect(mocks.publish).not.toHaveBeenCalled();
        await act(async () => decode.reject(new Error('Invalid image')));
        expect(await screen.findByRole('alert')).toBeInTheDocument();
        fireEvent.submit(submit.closest('form')!);
        expect(mocks.publish).not.toHaveBeenCalled();
        await userEvent.upload(input, new File(['photo'], 'valid.png', { type: 'image/png' }));
        await screen.findByRole('img', { name: 'Photo to publish' });
        expect(submit).toBeEnabled();
        expect(screen.queryByRole('alert')).not.toBeInTheDocument();
        expect(screen.getByLabelText('Caption')).toHaveValue('A mystery');
        expect(screen.getByLabelText('Latitude')).toHaveValue(48.8);
    });

    it.each(['success', 'failure'])(
        'ignores a stale decoder %s after replacing the selected photo',
        async (outcome) => {
            const decode = deferredBitmap();
            vi.mocked(createImageBitmap).mockReturnValueOnce(decode.promise);
            const { input, submit } = renderComposer();
            await userEvent.upload(input, new File(['first'], 'first.jpg', { type: 'image/jpeg' }));
            await userEvent.upload(input, new File(['second'], 'second.jpg', { type: 'image/jpeg' }));
            const preview = await screen.findByRole('img', { name: 'Photo to publish' });
            const image = bitmap(100, 100);
            await act(async () => {
                if (outcome === 'success') decode.resolve(image);
                else decode.reject(new Error('Stale failure'));
            });
            expect(preview).toHaveAttribute('width', '320');
            expect(submit).toBeEnabled();
            expect(screen.queryByRole('alert')).not.toBeInTheDocument();
            if (outcome === 'success') expect(image.close).toHaveBeenCalledTimes(1);
        },
    );

    it('hides the previous preview while decoding a replacement and after clearing the selection', async () => {
        const { input, submit } = renderComposer();
        await userEvent.upload(input, new File(['first'], 'first.jpg', { type: 'image/jpeg' }));
        await screen.findByRole('img', { name: 'Photo to publish' });
        const decode = deferredBitmap();
        vi.mocked(createImageBitmap).mockReturnValueOnce(decode.promise);
        await userEvent.upload(input, new File(['second'], 'second.jpg', { type: 'image/jpeg' }));
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
        expect(submit).toBeDisabled();
        fireEvent.change(input, { target: { files: [] } });
        const image = bitmap();
        await act(async () => decode.resolve(image));
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
        expect(screen.queryByRole('status')).not.toBeInTheDocument();
        expect(submit).toBeDisabled();
        expect(image.close).toHaveBeenCalledTimes(1);
    });

    it('closes a bitmap delivered after unmount without drawing it', async () => {
        const decode = deferredBitmap();
        vi.mocked(createImageBitmap).mockReturnValueOnce(decode.promise);
        const { input, unmount } = renderComposer();
        await userEvent.upload(input, new File(['photo'], 'place.jpg', { type: 'image/jpeg' }));
        unmount();
        const image = bitmap();
        await act(async () => decode.resolve(image));
        expect(image.close).toHaveBeenCalledTimes(1);
        expect(HTMLCanvasElement.prototype.getContext).not.toHaveBeenCalled();
    });

    it('releases the bitmap when canvas rendering is unavailable', async () => {
        const image = bitmap();
        vi.mocked(createImageBitmap).mockResolvedValue(image);
        vi.mocked(HTMLCanvasElement.prototype.getContext).mockReturnValue(null);
        const { input, submit } = renderComposer();
        await userEvent.upload(input, new File(['photo'], 'place.jpg', { type: 'image/jpeg' }));
        await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument());
        expect(image.close).toHaveBeenCalledTimes(1);
        expect(submit).toBeDisabled();
    });
});
