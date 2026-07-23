import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Camera from './Camera';

const mocks = vi.hoisted(() => ({
    post: vi.fn(),
    getUserMedia: vi.fn(),
    getCurrentPosition: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

function stubUserMedia() {
    const trackStop = vi.fn();
    const tracks = [{ stop: trackStop }] as unknown as MediaStreamTrack[];
    const stream = {
        getTracks: () => tracks,
        getVideoTracks: () => tracks,
    } as unknown as MediaStream;
    mocks.getUserMedia.mockResolvedValue(stream);
    return { stream, trackStop };
}

function stubGeolocation() {
    mocks.getCurrentPosition.mockImplementation((resolve: PositionCallback) =>
        resolve({
            coords: {
                latitude: 45.5,
                longitude: -73.6,
                accuracy: 10,
                altitude: null,
                altitudeAccuracy: null,
                heading: null,
                speed: null,
                toJSON: () => ({}),
            },
            timestamp: Date.now(),
            toJSON: () => ({}),
        }),
    );
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.getUserMedia.mockReset();
    mocks.post.mockReset();
    mocks.getCurrentPosition.mockReset();

    vi.stubGlobal('navigator', {
        mediaDevices: { getUserMedia: mocks.getUserMedia },
        geolocation: { getCurrentPosition: mocks.getCurrentPosition },
    });

    HTMLCanvasElement.prototype.getContext = vi.fn().mockReturnValue({
        drawImage: vi.fn(),
        clearRect: vi.fn(),
    } as unknown as CanvasRenderingContext2D);
    HTMLCanvasElement.prototype.toDataURL = vi.fn().mockReturnValue('data:image/jpeg;base64,abc123');

    Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, value: 640 });
    Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, value: 480 });

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
    vi.unstubAllGlobals();
});

describe('Camera component', () => {
    it('renders loading state while camera initializes', () => {
        mocks.getUserMedia.mockReturnValue(new Promise(() => {}));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        expect(screen.getByText('Loading camera...')).toBeInTheDocument();
    });

    it('shows error UI when camera access is denied', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => {
            expect(screen.getByText(/Camera access denied/i)).toBeInTheDocument();
        });
        expect(screen.getByRole('button', { name: 'Try Again' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
    });

    it('retries camera access when Try Again is clicked', async () => {
        mocks.getUserMedia.mockRejectedValueOnce(new DOMException('Permission denied', 'NotAllowedError'));
        stubUserMedia();
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Try Again' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Try Again' }));
        await waitFor(() => {
            expect(mocks.getUserMedia).toHaveBeenCalledTimes(2);
        });
    });

    it('opens file picker fallback from camera error state', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));
        await waitFor(() => {
            expect(screen.getByLabelText('Choose photo from device')).toBeInTheDocument();
        });
    });

    it('handles file selection via the file input', async () => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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
        const formData = mocks.post.mock.calls[0][1] as FormData;
        expect(formData.get('group_id')).toBe('group-1');
        expect(formData.get('lat')).toBe('45.5');
        expect(formData.get('long')).toBe('-73.6');
        const uploadedPhoto = formData.get('photo');
        expect(uploadedPhoto).toBeInstanceOf(Blob);
        expect((uploadedPhoto as Blob).type).toBe('image/png');
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
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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
        await waitFor(() => {
            expect(screen.getByRole('button', { name: 'Upload from device' })).toBeInTheDocument();
        });
        fireEvent.click(screen.getByRole('button', { name: 'Upload from device' }));

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

    it('stops camera stream on unmount', async () => {
        const { trackStop } = stubUserMedia();
        const { unmount } = render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);

        // Wait for getUserMedia to resolve so streamRef.current is populated.
        await waitFor(() => {
            expect(mocks.getUserMedia).toHaveBeenCalled();
        });

        // Let async effect settle.
        await act(async () => {
            await Promise.resolve();
        });

        // Unmount triggers useEffect cleanup → stopCamera → track.stop().
        unmount();
        expect(trackStop).toHaveBeenCalled();
    });

    it('does not start camera initialization after an immediate unmount', async () => {
        const { unmount } = render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        unmount();

        await act(async () => {
            await Promise.resolve();
        });

        expect(mocks.getUserMedia).not.toHaveBeenCalled();
    });
});
