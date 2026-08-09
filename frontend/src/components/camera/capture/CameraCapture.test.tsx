import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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

function stubGeolocation() {
    mocks.getCurrentPosition.mockImplementation((resolve: PositionCallback) =>
        resolve({ coords: { latitude: 45.5, longitude: -73.6 } } as GeolocationPosition),
    );
}

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

describe('Camera capture', () => {
    it('captures a ready camera frame and can retake it', async () => {
        stubUserMedia();
        Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
        const play = vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);

        await waitFor(() => expect(screen.getByRole('button', { name: 'Take photo' })).toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', { name: 'Take photo' }));
        expect(screen.getByAltText('Captured')).toBeInTheDocument();
        expect(document.querySelector('.camera-flash')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'Retake' }));
        await waitFor(() => expect(mocks.getUserMedia).toHaveBeenCalledTimes(2));
        expect(play).toHaveBeenCalled();
    });

    it('adds an editable text banner to a captured camera photo before upload', async () => {
        stubUserMedia();
        stubGeolocation();
        mocks.post.mockResolvedValue({ data: {} });
        Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
        vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
        const context = {
            beginPath: vi.fn(),
            closePath: vi.fn(),
            drawImage: vi.fn(),
            fill: vi.fn(),
            fillText: vi.fn(),
            lineTo: vi.fn(),
            measureText: vi.fn((text: string) => ({ width: text.length * 20 })),
            moveTo: vi.fn(),
            quadraticCurveTo: vi.fn(),
            restore: vi.fn(),
            save: vi.fn(),
            setTransform: vi.fn(),
        } as unknown as CanvasRenderingContext2D;
        HTMLCanvasElement.prototype.getContext = vi.fn().mockReturnValue(context);

        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => expect(screen.getByRole('button', { name: 'Take photo' })).toBeInTheDocument());
        fireEvent.click(screen.getByRole('button', { name: 'Take photo' }));
        fireEvent.click(screen.getByRole('button', { name: /text/i }));
        fireEvent.change(screen.getByPlaceholderText('Say something dangerous…'), {
            target: { value: 'CEO OF BAD IDEAS' },
        });
        expect(screen.getByText('CEO OF BAD IDEAS')).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: /Send/ }));
        await waitFor(() => expect(mocks.post).toHaveBeenCalledOnce());
        expect(context.fillText).toHaveBeenCalledWith('CEO OF BAD IDEAS', 320, expect.any(Number), 550.4);
    });
});
