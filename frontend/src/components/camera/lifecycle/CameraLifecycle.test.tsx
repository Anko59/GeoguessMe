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
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('Camera lifecycle', () => {
    it('renders loading state while camera initializes', () => {
        mocks.getUserMedia.mockReturnValue(new Promise(() => {}));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        expect(screen.getByText('Loading camera...')).toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Challenge options' })).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'Take photo' })).not.toBeInTheDocument();
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

    it.each([
        ['SecurityError', /Camera access denied/i],
        ['NotFoundError', /No camera was found/i],
        ['DevicesNotFoundError', /No camera was found/i],
        ['NotReadableError', /camera is busy or unavailable/i],
        ['TrackStartError', /camera is busy or unavailable/i],
        ['UnknownError', /camera could not be started/i],
    ])('maps %s camera failures to actionable guidance', async (name, expectedMessage) => {
        mocks.getUserMedia.mockRejectedValue(new DOMException('Camera failed', name));
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => expect(screen.getByText(expectedMessage)).toBeInTheDocument());
    });

    it('offers file upload when the media devices API is unavailable', async () => {
        vi.stubGlobal('navigator', {
            mediaDevices: undefined,
            geolocation: { getCurrentPosition: mocks.getCurrentPosition },
        });
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);

        await waitFor(() => {
            expect(screen.getByText(/Camera access denied or unavailable/i)).toBeInTheDocument();
        });
        expect(mocks.getUserMedia).not.toHaveBeenCalled();
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
        await useDeviceFallback();
        await waitFor(() => {
            expect(screen.getByLabelText('Choose photo from device')).toBeInTheDocument();
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

    it('shows a camera switch button and switches to the back camera', async () => {
        const camera = { kind: 'videoinput' as const };
        const enumerateDevices = vi
            .fn()
            .mockImplementationOnce(() => [camera])
            .mockImplementation(() => [camera, camera]);
        const { trackStop } = stubUserMedia();
        vi.stubGlobal('navigator', {
            mediaDevices: { getUserMedia: mocks.getUserMedia, enumerateDevices },
            geolocation: { getCurrentPosition: mocks.getCurrentPosition },
        });
        Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
        vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => expect(screen.getByRole('button', { name: 'Take photo' })).toBeInTheDocument());
        await Promise.resolve();
        const switchBtn = screen.getByLabelText(/switch to back camera/i);
        expect(trackStop).not.toHaveBeenCalled();
        fireEvent.click(switchBtn);
        await waitFor(() => expect(trackStop).toHaveBeenCalled());
        await waitFor(() => expect(document.querySelector('.camera-video')).not.toHaveClass('mirrored'));
        const constraints = mocks.getUserMedia.mock.calls[1][0] as MediaStreamConstraints;
        expect((constraints.video as { facingMode: string }).facingMode).toBe('environment');
    });
});
