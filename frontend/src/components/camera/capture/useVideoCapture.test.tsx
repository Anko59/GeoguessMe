import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useVideoCapture } from './useVideoCapture';

interface FakeRecorder {
    stream: MediaStream;
    state: RecordingState;
    ondataavailable: ((event: BlobEvent) => void) | null;
    onstop: ((event: Event) => void) | null;
    start: () => void;
    stop: () => void;
}

const recorder: { current: FakeRecorder | undefined } = { current: undefined };

function installRecorder() {
    recorder.current = undefined;
    vi.stubGlobal(
        'MediaRecorder',
        class {
            stream: MediaStream;
            state: RecordingState = 'inactive';
            ondataavailable: ((event: BlobEvent) => void) | null = null;
            onstop: ((event: Event) => void) | null = null;
            static isTypeSupported = () => true;
            // eslint-disable-next-line @typescript-eslint/no-unused-vars
            constructor(_stream: MediaStream, _options: MediaRecorderOptions) {
                this.stream = _stream;
                recorder.current = this as unknown as FakeRecorder;
            }
            start() {
                this.state = 'recording';
            }
            stop() {
                this.state = 'inactive';
                this.ondataavailable?.({ data: new Blob(['clip'], { type: 'video/webm' }) } as BlobEvent);
                this.onstop?.(new Event('stop'));
            }
        },
    );
}

// jsdom's MediaStream rejects the plain fake tracks used here; replace it with a
// minimal container so the hook can merge video and audio tracks for recording.
function installMediaStream() {
    vi.stubGlobal(
        'MediaStream',
        class {
            tracks: MediaStreamTrack[];
            constructor(input: MediaStreamTrack[] | MediaStream) {
                this.tracks = Array.isArray(input) ? input : [];
            }
            getTracks() {
                return this.tracks;
            }
        },
    );
}

function makeStream(stops: { stop: () => void }[], kind: 'video' | 'audio') {
    return {
        getTracks: () => stops,
        getVideoTracks: () => (kind === 'video' ? stops : []),
        getAudioTracks: () => (kind === 'audio' ? stops : []),
    } as unknown as MediaStream;
}

function Harness({
    videoStream,
    stillPressed,
    onComplete,
    mirror = false,
    videoElement = null,
}: {
    videoStream: MediaStream;
    stillPressed: boolean;
    onComplete: () => void;
    mirror?: boolean;
    videoElement?: HTMLVideoElement | null;
}) {
    const capture = useVideoCapture({ onError: vi.fn() });
    return (
        <>
            <button
                onClick={() =>
                    void capture.startHeldRecording(videoStream, () => stillPressed, onComplete, mirror, videoElement)
                }
            >
                start
            </button>
            <button onClick={capture.stopRecording}>stop</button>
            <output>{capture.recording ? 'recording' : 'idle'}</output>
        </>
    );
}

afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    // jsdom never implements canvas.captureStream; the mirror test installs it.
    delete (HTMLCanvasElement.prototype as unknown as Record<string, unknown>).captureStream;
});

describe('useVideoCapture', () => {
    it('requests only the microphone and records without touching the camera stream', async () => {
        installRecorder();
        installMediaStream();
        const videoStop = vi.fn();
        const audioStop = vi.fn();
        const videoStream = makeStream([{ stop: videoStop }], 'video');
        const getUserMedia = vi.fn().mockResolvedValue(makeStream([{ stop: audioStop }], 'audio'));
        vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

        const onComplete = vi.fn();
        render(<Harness videoStream={videoStream} stillPressed onComplete={onComplete} />);
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'start' }));
        });

        // The live camera preview is never re-requested; only the microphone is.
        expect(getUserMedia).toHaveBeenCalledTimes(1);
        expect(getUserMedia.mock.calls[0][0]).toEqual({ audio: true });
        await waitFor(() => expect(screen.getByText('recording')).toBeInTheDocument());
        expect(recorder.current?.state).toBe('recording');
        // The recorded stream merges the camera video track with the mic audio.
        expect(recorder.current?.stream.getTracks()).toHaveLength(2);
        expect(videoStop).not.toHaveBeenCalled();
        expect(onComplete).not.toHaveBeenCalled();

        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'stop' }));
        });
        expect(onComplete).toHaveBeenCalledTimes(1);
        // The microphone is freed when the clip ends.
        expect(audioStop).toHaveBeenCalledTimes(1);
    });

    it('records a video-only clip when the microphone is unavailable', async () => {
        installRecorder();
        installMediaStream();
        const videoStream = makeStream([{ stop: vi.fn() }], 'video');
        const getUserMedia = vi.fn().mockRejectedValue(new DOMException('denied', 'NotAllowedError'));
        vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

        render(<Harness videoStream={videoStream} stillPressed onComplete={vi.fn()} />);
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'start' }));
        });

        expect(getUserMedia).toHaveBeenCalledWith({ audio: true });
        await waitFor(() => expect(screen.getByText('recording')).toBeInTheDocument());
        expect(recorder.current?.stream.getTracks()).toHaveLength(1);
    });

    it('ignores a hold that is released before the microphone is granted', async () => {
        installRecorder();
        installMediaStream();
        const audioStop = vi.fn();
        const videoStream = makeStream([{ stop: vi.fn() }], 'video');
        const getUserMedia = vi.fn().mockResolvedValue(makeStream([{ stop: audioStop }], 'audio'));
        vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });

        render(<Harness videoStream={videoStream} stillPressed={false} onComplete={vi.fn()} />);
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'start' }));
        });

        // The would-be audio stream is released immediately; nothing is recorded.
        expect(audioStop).toHaveBeenCalledTimes(1);
        expect(screen.getByText('idle')).toBeInTheDocument();
        expect(recorder.current).toBeUndefined();
    });

    it('mirrors front-camera clips through a canvas capture so they match the preview', async () => {
        installRecorder();
        installMediaStream();
        const videoStop = vi.fn();
        const audioStop = vi.fn();
        const canvasVideoStop = vi.fn();
        const videoStream = makeStream([{ stop: videoStop }], 'video');
        const canvasStream = makeStream([{ stop: canvasVideoStop }], 'video');
        const getUserMedia = vi.fn().mockResolvedValue(makeStream([{ stop: audioStop }], 'audio'));
        vi.stubGlobal('navigator', { mediaDevices: { getUserMedia } });
        Object.defineProperty(HTMLCanvasElement.prototype, 'captureStream', {
            configurable: true,
            value: vi.fn(() => canvasStream),
        });
        vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({
            setTransform: vi.fn(),
            drawImage: vi.fn(),
        } as unknown as CanvasRenderingContext2D);
        vi.stubGlobal(
            'requestAnimationFrame',
            vi.fn(() => 1),
        );
        vi.stubGlobal('cancelAnimationFrame', vi.fn());

        const onComplete = vi.fn();
        render(
            <Harness
                videoStream={videoStream}
                stillPressed
                onComplete={onComplete}
                mirror
                videoElement={{ videoWidth: 640, videoHeight: 480, readyState: 2 } as unknown as HTMLVideoElement}
            />,
        );
        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'start' }));
        });

        // The live camera stream is never re-requested; the recorder consumes
        // the mirrored canvas video track merged with the microphone.
        expect(getUserMedia).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(screen.getByText('recording')).toBeInTheDocument());
        expect(recorder.current?.stream.getTracks()).toHaveLength(2);
        expect(recorder.current?.stream.getTracks()[0]).toBe(canvasStream.getTracks()[0]);
        expect(videoStop).not.toHaveBeenCalled();

        await act(async () => {
            fireEvent.click(screen.getByRole('button', { name: 'stop' }));
        });
        expect(onComplete).toHaveBeenCalledTimes(1);
        expect(audioStop).toHaveBeenCalledTimes(1);
        // The mirrored canvas source is released together with the clip.
        expect(canvasVideoStop).toHaveBeenCalledTimes(1);
    });
});
