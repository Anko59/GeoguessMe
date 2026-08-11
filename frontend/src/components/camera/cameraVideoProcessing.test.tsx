import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import Camera from './Camera';
import { isProcessingJob, uploadPhoto } from './cameraUtils';
import type { MediaProcessingJob } from '../../types';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    post: vi.fn(),
    getUserMedia: vi.fn(),
    getCurrentPosition: vi.fn(),
}));

vi.mock('../../api', () => ({
    default: { get: mocks.get, post: mocks.post },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
    getAccessToken: () => 'token',
}));

// A controllable recorder: stop() emits one clip and the stop event, matching
// the real MediaRecorder contract the capture hook relies on.
class FakeMediaRecorder {
    static instance: FakeMediaRecorder | null = null;
    static isTypeSupported = vi.fn(() => true);
    state: RecordingState = 'inactive';
    ondataavailable: ((event: BlobEvent) => void) | null = null;
    onerror: ((event: Event) => void) | null = null;
    onstop: ((event: Event) => void) | null = null;
    mimeType = 'video/webm;codecs=vp8,opus';

    constructor(stream: MediaStream, options: MediaRecorderOptions) {
        void stream;
        void options;
        FakeMediaRecorder.instance = this;
    }

    start() {
        this.state = 'recording';
    }

    stop() {
        this.state = 'inactive';
        this.ondataavailable?.({ data: new Blob(['clip'], { type: 'video/webm' }) } as BlobEvent);
        this.onstop?.(new Event('stop'));
    }
}

function stubUserMedia() {
    const trackStop = vi.fn();
    const tracks = [{ stop: trackStop }] as unknown as MediaStreamTrack[];
    const stream = {
        getTracks: () => tracks,
        getVideoTracks: () => tracks,
        getAudioTracks: () => [] as MediaStreamTrack[],
    } as unknown as MediaStream;
    mocks.getUserMedia.mockResolvedValue(stream);
    return { stream, trackStop };
}

function stubGeolocation() {
    mocks.getCurrentPosition.mockImplementation((resolve: PositionCallback) =>
        resolve({ coords: { latitude: 45.5, longitude: -73.6 } } as GeolocationPosition),
    );
}

function queuedJob(): MediaProcessingJob {
    return { id: 'job-1', kind: 'challenge', status: 'queued', queued_at: '2026-08-10T12:00:00Z' };
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
    // jsdom exposes captureStream as a no-op function returning a non-stream;
    // force the raw-stream recording fallback so the mirrored-canvas path is skipped.
    Object.defineProperty(HTMLCanvasElement.prototype, 'captureStream', {
        configurable: true,
        value: undefined,
    });

    Object.defineProperty(HTMLVideoElement.prototype, 'videoWidth', { configurable: true, value: 640 });
    Object.defineProperty(HTMLVideoElement.prototype, 'videoHeight', { configurable: true, value: 480 });
    Object.defineProperty(HTMLVideoElement.prototype, 'srcObject', {
        configurable: true,
        writable: true,
        value: null,
    });
    Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
    vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);

    vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
    vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:recorded-video'), revokeObjectURL: vi.fn() });
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    FakeMediaRecorder.instance = null;
});

// Wait the real hold-to-record duration so the 300ms hold timer fires under
// real timers (waitFor cannot advance vitest fake timers).
async function holdToRecord() {
    const captureButton = await waitFor(() => screen.getByRole('button', { name: 'Take photo' }));
    fireEvent.pointerDown(captureButton);
    await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 350));
    });
    fireEvent.pointerUp(captureButton);
    await waitFor(() => expect(screen.getByLabelText('Recorded video preview')).toBeInTheDocument());
}

describe('Camera video processing flow', () => {
    it('accepts a recorded video (202), shows processing, and navigates once ready', async () => {
        stubUserMedia();
        stubGeolocation();
        const queued = queuedJob();
        const ready = { ...queued, status: 'ready' as const };
        mocks.get.mockResolvedValueOnce({ data: queued }).mockResolvedValueOnce({ data: ready });
        const onUploadComplete = vi.fn();

        mocks.post.mockResolvedValue({ data: queued, status: 202 });
        render(<Camera groupID="group-1" onUploadComplete={onUploadComplete} />);
        await holdToRecord();
        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        // The 202 upload starts polling; the immediate poll is still queued.
        await waitFor(() => expect(screen.getByText(/Processing video/)).toBeInTheDocument());
        expect(onUploadComplete).not.toHaveBeenCalled();

        // The second poll (1s later) observes ready and resolves the upload.
        await waitFor(() => expect(onUploadComplete).toHaveBeenCalledTimes(1), { timeout: 4000 });
        expect(mocks.get).toHaveBeenCalledWith('/media-processing/job-1', expect.anything());
    });

    it('shows a friendly error when processing fails and restores the retake affordance', async () => {
        stubUserMedia();
        stubGeolocation();
        const queued = queuedJob();
        const failed = { ...queued, status: 'failed' as const, error_code: 'too_long' };
        mocks.get.mockResolvedValueOnce({ data: queued }).mockResolvedValueOnce({ data: failed });
        const onUploadComplete = vi.fn();

        mocks.post.mockResolvedValue({ data: queued, status: 202 });
        render(<Camera groupID="group-1" onUploadComplete={onUploadComplete} />);
        await holdToRecord();
        fireEvent.click(screen.getByRole('button', { name: /Send/ }));

        await waitFor(() => expect(screen.getByText(/Processing video/)).toBeInTheDocument());

        // The second poll observes the failure; the friendly copy never leaks
        // the error code itself and retake/send come back.
        await waitFor(() => expect(screen.getByText(/30 seconds or shorter/)).toBeInTheDocument(), { timeout: 4000 });
        expect(screen.queryByText('too_long')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'Retake' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /Send/ })).toBeInTheDocument();
        expect(onUploadComplete).not.toHaveBeenCalled();

        // Polling stopped after the terminal status: exactly two requests.
        expect(mocks.get.mock.calls.length).toBe(2);
    });
});

describe('uploadPhoto response branching', () => {
    it('returns the processing job on 202 and null on 201', async () => {
        const position = { coords: { latitude: 45.5, longitude: -73.6 } } as GeolocationPosition;
        const queued = queuedJob();
        mocks.post.mockResolvedValue({ data: queued, status: 202 });
        const videoJob = await uploadPhoto(
            new Blob(['clip'], { type: 'video/webm' }),
            'capture.webm',
            ['group-1'],
            position,
        );
        expect(videoJob).toEqual(queued);
        expect(mocks.post).toHaveBeenCalledWith('/photo/upload', expect.any(FormData));

        mocks.post.mockResolvedValue({ data: { id: 'challenge-1', group_id: 'group-1' }, status: 201 });
        const imageJob = await uploadPhoto(
            new Blob(['img'], { type: 'image/jpeg' }),
            'capture.jpg',
            ['group-1'],
            position,
        );
        expect(imageJob).toBeNull();
    });

    it('classifies job-shaped bodies without false positives', () => {
        expect(isProcessingJob({ id: 'j', kind: 'challenge', status: 'queued', queued_at: 'x' })).toBe(true);
        expect(
            isProcessingJob({ id: 'j', kind: 'chat', status: 'failed', queued_at: 'x', error_code: 'timeout' }),
        ).toBe(true);
        expect(isProcessingJob({ id: 'c', group_id: 'g' })).toBe(false);
        expect(isProcessingJob({})).toBe(false);
        expect(isProcessingJob(null)).toBe(false);
        expect(isProcessingJob('job')).toBe(false);
    });
});
