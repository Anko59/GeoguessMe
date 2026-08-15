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

describe('Camera lens and filter UI', () => {
    it('toggles filter picker visibility when the filter toggle is clicked', async () => {
        stubUserMedia();
        Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
        vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
        render(<Camera groupID="group-1" onUploadComplete={vi.fn()} />);
        await waitFor(() => expect(screen.getByRole('button', { name: 'Take photo' })).toBeInTheDocument());
        const toggle = screen.getByRole('button', { name: 'Hide lenses' });
        expect(toggle).toHaveAttribute('aria-expanded', 'true');
        fireEvent.click(toggle);
        expect(screen.getByRole('button', { name: 'Show lenses' })).toHaveAttribute('aria-expanded', 'false');
    });
});
