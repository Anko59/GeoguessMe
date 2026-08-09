import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useCallback } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useCameraSession } from './useCameraSession';

const mocks = vi.hoisted(() => ({ getUserMedia: vi.fn() }));

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (error: unknown) => void;
    const promise = new Promise<T>((res, rej) => {
        resolve = res;
        reject = rej;
    });
    return { promise, resolve, reject };
}

function makeStream() {
    const trackStop = vi.fn();
    const tracks = [{ stop: trackStop }] as unknown as MediaStreamTrack[];
    return {
        stream: { getTracks: () => tracks } as unknown as MediaStream,
        trackStop,
    };
}

type SessionApi = ReturnType<typeof useCameraSession>;

interface HarnessOptions {
    onReady?: (video: HTMLVideoElement, width: number, height: number) => void;
    setError?: (message: string) => void;
}

function renderHarness(options: HarnessOptions = {}) {
    const api: { current: SessionApi | null } = { current: null };
    const onReady = options.onReady;
    const setError = options.setError;
    function Harness() {
        const stableOnReady = useCallback(
            (video: HTMLVideoElement, width: number, height: number) => onReady?.(video, width, height),
            [],
        );
        const stableOnReset = useCallback(() => undefined, []);
        const session = useCameraSession({
            onReset: stableOnReset,
            onReady: stableOnReady,
            setError: useCallback((message: string) => setError?.(message), []),
        });
        const { videoRef, startCamera, stopCamera, switchCamera } = session;
        api.current = session;
        return (
            <div>
                <video ref={videoRef} />
                <button onClick={() => void startCamera()}>start</button>
                <button onClick={stopCamera}>stop</button>
                <button onClick={switchCamera}>switch</button>
            </div>
        );
    }
    const view = render(<Harness />);
    return { api, unmount: view.unmount };
}

beforeEach(() => {
    vi.clearAllMocks();
    mocks.getUserMedia.mockReset();

    vi.stubGlobal('navigator', {
        mediaDevices: { getUserMedia: mocks.getUserMedia },
        geolocation: undefined,
    });

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

describe('useCameraSession', () => {
    it('starts the camera on mount and attaches the stream to the video element', async () => {
        const { stream } = makeStream();
        mocks.getUserMedia.mockResolvedValue(stream);
        renderHarness();

        await waitFor(() => {
            const video = document.querySelector('video');
            expect(video?.srcObject).toBe(stream);
        });
        const constraints = mocks.getUserMedia.mock.calls[0][0] as MediaStreamConstraints;
        expect((constraints.video as { facingMode: string }).facingMode).toBe('user');
        expect((constraints.video as { width: { ideal: number } }).width.ideal).toBe(1280);
        expect(constraints.audio).toBe(false);
    });

    it('stops the superseded stream when a newer attempt starts before it resolves', async () => {
        const pending: Array<ReturnType<typeof deferred<MediaStream>>> = [];
        mocks.getUserMedia.mockImplementation(() => {
            const d = deferred<MediaStream>();
            pending.push(d);
            return d.promise;
        });
        renderHarness();

        await waitFor(() => expect(mocks.getUserMedia).toHaveBeenCalledTimes(1));
        fireEvent.click(screen.getByRole('button', { name: 'start' }));
        await waitFor(() => expect(mocks.getUserMedia).toHaveBeenCalledTimes(2));

        const first = makeStream();
        const second = makeStream();
        await act(async () => {
            pending[0].resolve(first.stream);
            await Promise.resolve();
        });
        // Stale attempt: its stream must be stopped, never attached.
        expect(first.trackStop).toHaveBeenCalled();

        await act(async () => {
            pending[1].resolve(second.stream);
            await Promise.resolve();
        });
        await waitFor(() => {
            const video = document.querySelector('video');
            expect(video?.srcObject).toBe(second.stream);
        });
        expect(second.trackStop).not.toHaveBeenCalled();
    });

    it('stops the active stream on unmount', async () => {
        const { stream, trackStop } = makeStream();
        mocks.getUserMedia.mockResolvedValue(stream);
        const { unmount } = renderHarness();

        await waitFor(() => {
            expect(document.querySelector('video')?.srcObject).toBe(stream);
        });
        unmount();
        expect(trackStop).toHaveBeenCalled();
    });

    it('never attaches a stream that resolves after unmount', async () => {
        const d = deferred<MediaStream>();
        mocks.getUserMedia.mockReturnValue(d.promise);
        const { unmount } = renderHarness();

        await waitFor(() => expect(mocks.getUserMedia).toHaveBeenCalledTimes(1));
        unmount();

        const { stream, trackStop } = makeStream();
        await act(async () => {
            d.resolve(stream);
            await Promise.resolve();
        });
        expect(trackStop).toHaveBeenCalled();
    });

    it('maps camera permission errors to an actionable message', async () => {
        const setError = vi.fn();
        mocks.getUserMedia.mockRejectedValue(new DOMException('Permission denied', 'NotAllowedError'));
        renderHarness({ setError });

        await waitFor(() => {
            expect(setError).toHaveBeenCalledWith('Camera access denied. Allow camera permissions and try again.');
        });
    });

    it('reports the missing media devices API without requesting a stream', async () => {
        const setError = vi.fn();
        vi.stubGlobal('navigator', { mediaDevices: undefined, geolocation: undefined });
        renderHarness({ setError });

        await waitFor(() => {
            expect(setError).toHaveBeenCalledWith(
                'Camera access denied or unavailable. Enable camera permissions or upload a photo from your device.',
            );
        });
        expect(mocks.getUserMedia).not.toHaveBeenCalled();
    });

    it('fires onReady once per attempt once the video can play', async () => {
        const { stream } = makeStream();
        mocks.getUserMedia.mockResolvedValue(stream);
        Object.defineProperty(HTMLVideoElement.prototype, 'readyState', { configurable: true, value: 2 });
        vi.spyOn(HTMLMediaElement.prototype, 'play').mockResolvedValue(undefined);
        const onReady = vi.fn();
        renderHarness({ onReady });

        await waitFor(() => expect(onReady).toHaveBeenCalledTimes(1));
        const video = document.querySelector('video');
        expect(onReady.mock.calls[0][0]).toBe(video);
        expect(onReady.mock.calls[0][1]).toBe(640);
        expect(onReady.mock.calls[0][2]).toBe(480);
    });

    it('restarts with the environment camera on switch and stops the old stream', async () => {
        const first = makeStream();
        const second = makeStream();
        mocks.getUserMedia.mockResolvedValueOnce(first.stream).mockResolvedValueOnce(second.stream);
        const enumerateDevices = vi
            .fn()
            .mockResolvedValue([{ kind: 'videoinput' }, { kind: 'videoinput' }, { kind: 'audioinput' }]);
        vi.stubGlobal('navigator', {
            mediaDevices: { getUserMedia: mocks.getUserMedia, enumerateDevices },
            geolocation: undefined,
        });

        renderHarness();
        await waitFor(() => {
            expect(document.querySelector('video')?.srcObject).toBe(first.stream);
        });
        expect(first.trackStop).not.toHaveBeenCalled();

        fireEvent.click(screen.getByRole('button', { name: 'switch' }));

        await waitFor(() => expect(mocks.getUserMedia).toHaveBeenCalledTimes(2));
        expect(first.trackStop).toHaveBeenCalled();
        const constraints = mocks.getUserMedia.mock.calls[1][0] as MediaStreamConstraints;
        expect((constraints.video as { facingMode: string }).facingMode).toBe('environment');
        await waitFor(() => {
            expect(document.querySelector('video')?.srcObject).toBe(second.stream);
        });
        expect(second.trackStop).not.toHaveBeenCalled();
    });
});
