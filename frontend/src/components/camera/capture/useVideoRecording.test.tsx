import { fireEvent, render, screen } from '@testing-library/react';
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
