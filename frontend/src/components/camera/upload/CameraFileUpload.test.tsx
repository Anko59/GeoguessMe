import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Camera from '../Camera';
const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    getUserMedia: vi.fn(),
    getCurrentPosition: vi.fn(),
}));

vi.mock('../../../api', () => ({
    default: { get: mocks.get, post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

function stubGeolocation() {
    mocks.getCurrentPosition.mockImplementation((resolve: PositionCallback) =>
        resolve({ coords: { latitude: 45.5, longitude: -73.6 } } as GeolocationPosition),
    );
}

async function useDeviceFallback() {
    fireEvent.click(await waitFor(() => screen.getByRole('button', { name: 'Upload from device' })));
} // opens the camera-error file fallback

beforeEach(() => {
    vi.clearAllMocks();
    [mocks.get, mocks.post, mocks.getUserMedia, mocks.getCurrentPosition].forEach((mock) => mock.mockReset());

    vi.stubGlobal('navigator', {
        mediaDevices: { getUserMedia: mocks.getUserMedia },
        geolocation: { getCurrentPosition: mocks.getCurrentPosition },
    });

    HTMLCanvasElement.prototype.getContext = vi.fn().mockReturnValue({
        drawImage: vi.fn(),
        clearRect: vi.fn(),
        setTransform: vi.fn(),
    } as unknown as CanvasRenderingContext2D);
    HTMLCanvasElement.prototype.toDataURL = vi.fn().mockReturnValue('data:image/jpeg;base64,abc123');

    Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, value: 640 });
    Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, value: 480 });
    Object.defineProperty(HTMLVideoElement.prototype, 'srcObject', {
        configurable: true,
        writable: true,
        value: null,
    });

    vi.stubGlobal(
        'FileReader',
        class {
            result: string | null = null;
            onload: ((ev: ProgressEvent<FileReader>) => void) | null = null;
            onerror: (() => void) | null = null;
            // eslint-disable-next-line @typescript-eslint/no-unused-vars
            readAsDataURL(_blob: Blob) {
                const encoded = btoa('mock-image-data');
                this.result = 'data:image/png;base64,' + encoded;
                Promise.resolve().then(() => this.onload?.({} as ProgressEvent<FileReader>));
            }
        },
    );
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('Camera file and upload flows', () => {
    it('handles file selection via the file input', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });
        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });
        // Retake in file mode clears the preview and shows the file picker again.
        fireEvent.click(screen.getByRole('button', { name: 'Retake' }));
        await waitFor(() => {
            expect(screen.getByLabelText('Choose photo from device')).toBeInTheDocument();
        });
    });

    it('keeps unsupported image files usable while disabling 3D preparation', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        fireEvent.change(input, { target: { files: [] } });
        expect(screen.queryByAltText('Captured')).not.toBeInTheDocument();
        await act(async () => {
            fireEvent.change(input, {
                target: { files: [new File(['animated-image'], 'photo.gif', { type: 'image/gif' })] },
            });
        });

        await waitFor(() => expect(screen.getByAltText('Captured')).toBeInTheDocument());
        expect(screen.getByText(/3D lenses support JPEG, PNG, and WebP/i)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Cyber visor' }));
        expect(screen.getByRole('button', { name: 'Cyber visor' })).toHaveAttribute('aria-pressed', 'true');
        fireEvent.click(screen.getByRole('button', { name: 'Original' }));
        expect(screen.getByRole('button', { name: 'Original' })).toHaveAttribute('aria-pressed', 'true');
    });

    it('uploads the original file when image filtering cannot prepare it', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        stubGeolocation();
        mocks.post.mockResolvedValue({ data: {} });
        const onUploadComplete = vi.fn();
        vi.stubGlobal(
            'Image',
            class {
                onerror: ((event: Event) => void) | null = null;

                set src(_value: string) {
                    Promise.resolve().then(() => this.onerror?.(new Event('error')));
                }
            },
        );

        render(<Camera groupID="group-1" onUploadComplete={onUploadComplete} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });

        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => {
            expect(mocks.post).toHaveBeenCalled();
        });
        expect(mocks.getCurrentPosition).toHaveBeenCalledOnce();
        expect(mocks.getCurrentPosition).toHaveBeenCalledWith(expect.any(Function), expect.any(Function), {
            enableHighAccuracy: false,
            timeout: 10_000,
            maximumAge: 60_000,
        });
        const formData = mocks.post.mock.calls[0][1] as FormData;
        expect(formData.getAll('group_ids')).toEqual(['group-1']);
        expect(formData.get('hide_location')).toBe('false');
        expect(formData.get('lat')).toBe('45.5');
        expect(formData.get('long')).toBe('-73.6');
        const uploadedPhoto = formData.get('photo');
        expect(uploadedPhoto).toBeInstanceOf(Blob);
        expect((uploadedPhoto as Blob).type).toBe('image/png');
        expect(onUploadComplete).toHaveBeenCalled();
    });

    it('lets the user pick target groups and hide the location before sending', async () => {
        const groups = [
            { id: 'group-1', name: 'Paris', code: 'ABC123' },
            { id: 'group-2', name: 'Berlin', code: 'DEF456' },
        ];
        mocks.get.mockResolvedValueOnce({ data: groups }).mockResolvedValue({ data: {} });
        mocks.getUserMedia.mockRejectedValue(new DOMException('denied', 'NotAllowedError'));
        mocks.post.mockResolvedValue({ data: {} });
        stubGeolocation();
        const onUploadComplete = vi.fn();
        render(<Camera groupID="group-1" onUploadComplete={onUploadComplete} />);
        await useDeviceFallback();
        await act(async () => {
            fireEvent.change(screen.getByLabelText('Choose photo from device'), {
                target: { files: [new File(['image'], 'upload.jpg', { type: 'image/jpeg' })] },
            });
        });
        await waitFor(() => expect(screen.getByAltText('Captured')).toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', { name: 'Challenge options' }));
        await waitFor(() => expect(mocks.get).toHaveBeenCalledWith('/user/groups'));
        fireEvent.click(screen.getByLabelText('Berlin'));
        fireEvent.click(screen.getByLabelText(/Hide my location/));
        fireEvent.click(screen.getByRole('button', { name: 'Done' }));
        fireEvent.click(screen.getByRole('button', { name: /Send/ }));
        await waitFor(() => expect(mocks.post).toHaveBeenCalled());
        const formData = mocks.post.mock.calls[0][1] as FormData;
        expect(formData.getAll('group_ids')).toEqual(['group-1', 'group-2']);
        expect(formData.get('hide_location')).toBe('true');
        expect(onUploadComplete).toHaveBeenCalled();
    });

    it('shows error when geolocation is denied during upload', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        mocks.getCurrentPosition.mockImplementation((_resolve: PositionCallback, reject: PositionErrorCallback) => {
            const err = new Error('User denied Geolocation');
            Object.assign(err, {
                code: 1,
                PERMISSION_DENIED: 1,
                POSITION_UNAVAILABLE: 2,
                TIMEOUT: 3,
            });
            reject?.(err as unknown as GeolocationPositionError);
        });

        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });

        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => {
            expect(screen.getByText(/Unable to retrieve location/i)).toBeInTheDocument();
        });
    });

    it('shows error when geolocation is not supported', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        vi.stubGlobal('navigator', {
            mediaDevices: { getUserMedia: mocks.getUserMedia },
            geolocation: undefined,
        });

        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });

        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => {
            expect(screen.getByText(/Unable to retrieve location/i)).toBeInTheDocument();
        });
    });

    it('shows API error when upload fails', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        stubGeolocation();
        mocks.post.mockRejectedValue(new Error('Server error'));

        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });

        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => {
            expect(screen.getByText('Server error')).toBeInTheDocument();
        });
    });

    it('disables buttons while uploading', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        stubGeolocation();
        mocks.post.mockReturnValue(new Promise(() => {}));

        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await useDeviceFallback();

        const file = new File(['fake-image'], 'photo.png', { type: 'image/png' });
        const input = screen.getByLabelText('Choose photo from device') as HTMLInputElement;
        await act(async () => {
            fireEvent.change(input, { target: { files: [file] } });
        });

        await waitFor(() => {
            expect(screen.getByAltText('Captured')).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Sending...' })).toBeDisabled();
            expect(screen.getByRole('button', { name: 'Retake' })).toBeDisabled();
        });
    });
});
