import { act, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useVideoRecording } from './useVideoRecording';

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

function Recorder() {
    const recording = useVideoRecording(vi.fn());
    return (
        <>
            <button onClick={() => recording.startRecording({} as MediaStream, vi.fn())}>Start</button>
            <button onClick={recording.stopRecording}>Stop</button>
            {recording.recordedVideo && <output>{recording.recordedVideo.blob.type}</output>}
        </>
    );
}

afterEach(() => vi.unstubAllGlobals());

describe('useVideoRecording', () => {
    it('ignores delayed final events after unmount without stopping borrowed preview tracks', () => {
        class DelayedRecorder extends FakeMediaRecorder {
            override stop() {
                this.state = 'inactive';
            }
        }
        vi.stubGlobal('MediaRecorder', DelayedRecorder);
        const createObjectURL = vi.fn(() => 'blob:late');
        vi.stubGlobal('URL', { createObjectURL, revokeObjectURL: vi.fn() });
        const onError = vi.fn();
        const onComplete = vi.fn();
        const onSettled = vi.fn();
        const stop = vi.fn();
        const { result, unmount } = renderHook(() => useVideoRecording(onError));
        act(() => {
            result.current.startRecording(
                { getTracks: () => [{ stop }] } as unknown as MediaStream,
                onComplete,
                onSettled,
            );
        });
        const recorder = FakeMediaRecorder.instance!;
        unmount();
        act(() => {
            recorder.ondataavailable?.({ data: new Blob(['late']) } as BlobEvent);
            recorder.onstop?.(new Event('stop'));
            recorder.onerror?.(new Event('error'));
        });
        expect(onSettled).toHaveBeenCalledTimes(1);
        expect(createObjectURL).not.toHaveBeenCalled();
        expect(onComplete).not.toHaveBeenCalled();
        expect(onError).not.toHaveBeenCalled();
        expect(stop).not.toHaveBeenCalled();
    });

    it('revokes each owned URL once on replacement, discard, and unmount', () => {
        vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
        const revokeObjectURL = vi.fn();
        vi.stubGlobal('URL', {
            createObjectURL: vi
                .fn()
                .mockReturnValueOnce('blob:first')
                .mockReturnValueOnce('blob:second')
                .mockReturnValueOnce('blob:third'),
            revokeObjectURL,
        });
        const { result, unmount } = renderHook(() => useVideoRecording(vi.fn()));
        act(() => {
            result.current.startRecording({} as MediaStream, vi.fn());
            result.current.stopRecording();
        });
        act(() => {
            result.current.startRecording({} as MediaStream, vi.fn());
            result.current.stopRecording();
        });
        act(() => result.current.discardRecording());
        act(() => {
            result.current.startRecording({} as MediaStream, vi.fn());
            result.current.stopRecording();
        });
        unmount();
        expect(revokeObjectURL.mock.calls).toEqual([['blob:first'], ['blob:second'], ['blob:third']]);
    });

    it('rejects final events from a superseded recording generation', () => {
        class DelayedRecorder extends FakeMediaRecorder {
            override stop() {
                this.state = 'inactive';
            }
        }
        vi.stubGlobal('MediaRecorder', DelayedRecorder);
        const createObjectURL = vi.fn(() => 'blob:current');
        vi.stubGlobal('URL', { createObjectURL, revokeObjectURL: vi.fn() });
        const onComplete = vi.fn();
        const onSettled = vi.fn();
        const { result } = renderHook(() => useVideoRecording(vi.fn()));
        act(() => result.current.startRecording({} as MediaStream, onComplete, onSettled));
        const old = FakeMediaRecorder.instance!;
        act(() => result.current.startRecording({} as MediaStream, onComplete, onSettled));
        act(() => {
            old.ondataavailable?.({ data: new Blob(['old']) } as BlobEvent);
            old.onstop?.(new Event('stop'));
        });
        expect(result.current.recording).toBe(true);
        expect(onSettled).toHaveBeenCalledTimes(1);
        expect(onComplete).not.toHaveBeenCalled();
        expect(createObjectURL).not.toHaveBeenCalled();
    });
    it('records a browser-supported WebM clip and exposes it for upload', () => {
        vi.stubGlobal('MediaRecorder', FakeMediaRecorder);
        vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:recorded-video'), revokeObjectURL: vi.fn() });
        render(<Recorder />);

        fireEvent.click(screen.getByRole('button', { name: 'Start' }));
        expect(FakeMediaRecorder.instance?.state).toBe('recording');
        fireEvent.click(screen.getByRole('button', { name: 'Stop' }));

        expect(screen.getByText(/^video\/webm/)).toBeInTheDocument();
    });

    it('uses the container MIME emitted by the recorder', () => {
        class RecorderWithContainerMIME extends FakeMediaRecorder {
            override mimeType = 'video/mp4';

            override stop() {
                this.state = 'inactive';
                this.ondataavailable?.({ data: new Blob(['clip'], { type: 'video/mp4' }) } as BlobEvent);
                this.onstop?.(new Event('stop'));
            }
        }
        vi.stubGlobal('MediaRecorder', RecorderWithContainerMIME);
        vi.stubGlobal('URL', { createObjectURL: vi.fn(() => 'blob:recorded-video'), revokeObjectURL: vi.fn() });
        render(<Recorder />);

        fireEvent.click(screen.getByRole('button', { name: 'Start' }));
        fireEvent.click(screen.getByRole('button', { name: 'Stop' }));

        expect(screen.getByText('video/mp4')).toBeInTheDocument();
    });
});
