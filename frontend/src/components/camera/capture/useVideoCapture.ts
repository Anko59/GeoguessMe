import { useCallback, useEffect, useRef } from 'react';
import { useVideoRecording, type RecordedVideo } from './useVideoRecording';

export type { RecordedVideo };

interface UseVideoCaptureOptions {
    onError: (message: string) => void;
}

interface MirroredCanvasSource {
    stream: MediaStream;
    stop: () => void;
}

const VIDEO_CAPTURE_FPS = 30;
const VIDEO_CAPTURE_FRAME_INTERVAL_MS = 1000 / VIDEO_CAPTURE_FPS;

type CanvasCaptureTrack = MediaStreamTrack & {
    requestFrame?: () => void;
};

/** Mirrors a live camera feed horizontally into a canvas and captures that
 *  canvas, so front-camera clips come out exactly like the mirrored preview
 *  instead of the raw sensor feed. The video element keeps playing normally;
 *  drawImage always reads its unflipped bitmap, so the flip is applied here.
 *  Returns null on browsers without canvas.captureStream so the caller falls
 *  back to recording the raw stream. */
function createMirroredCanvasSource(video: HTMLVideoElement): MirroredCanvasSource | null {
    if (typeof HTMLCanvasElement.prototype.captureStream !== 'function') return null;
    const canvas = document.createElement('canvas');
    canvas.width = video.videoWidth || 1280;
    canvas.height = video.videoHeight || 720;
    const context = canvas.getContext('2d');
    // Manual frame requests keep the mirrored canvas below the backend's hard
    // 30 fps input bound even when a browser's automatic canvas capture emits
    // more frames than the requested rate. Fall back to the automatic API for
    // browsers that do not expose requestFrame().
    let stream = canvas.captureStream(0);
    let videoTrack = stream.getVideoTracks()[0] as CanvasCaptureTrack | undefined;
    const manualCapture = typeof videoTrack?.requestFrame === 'function';
    if (!manualCapture) {
        stream.getTracks().forEach((track) => track.stop());
        stream = canvas.captureStream(VIDEO_CAPTURE_FPS);
        videoTrack = stream.getVideoTracks()[0] as CanvasCaptureTrack | undefined;
    }
    let frameID = 0;
    let lastFrameAt = Number.NEGATIVE_INFINITY;
    const drawFrame = (timestamp: number) => {
        if (
            context &&
            video.readyState >= 2 &&
            video.videoWidth > 0 &&
            timestamp - lastFrameAt >= VIDEO_CAPTURE_FRAME_INTERVAL_MS
        ) {
            context.setTransform(-1, 0, 0, 1, canvas.width, 0);
            context.drawImage(video, 0, 0, canvas.width, canvas.height);
            lastFrameAt = timestamp;
            videoTrack?.requestFrame?.();
        }
        frameID = requestAnimationFrame(drawFrame);
    };
    frameID = requestAnimationFrame(drawFrame);
    return {
        stream,
        stop: () => {
            cancelAnimationFrame(frameID);
            stream.getTracks().forEach((track) => track.stop());
        },
    };
}

// Records held video clips from an already-running camera stream. The
// microphone is acquired as a separate stream and merged with the live video
// track, so the camera preview is never restarted: re-requesting the camera
// mid-hold re-triggered the loading spinner, blanked the preview on mobile, and
// often captured a stream whose video track was not yet flowing, producing an
// empty, unplayable clip. If the microphone is unavailable, a video-only clip is
// recorded so the feature still works.
export function useVideoCapture({ onError }: UseVideoCaptureOptions) {
    const { recordedVideo, recording, startRecording, stopRecording, discardRecording } = useVideoRecording(onError);
    const cleanupRef = useRef<(() => void) | null>(null);
    const generationRef = useRef(0);
    const disposedRef = useRef(false);

    const releaseOwnedResources = useCallback(() => {
        generationRef.current += 1;
        cleanupRef.current?.();
        cleanupRef.current = null;
    }, []);

    const startHeldRecording = useCallback(
        async (
            videoStream: MediaStream,
            isStillPressed: () => boolean,
            onComplete: () => void,
            mirror = false,
            videoElement: HTMLVideoElement | null = null,
        ) => {
            if (disposedRef.current) return;
            releaseOwnedResources();
            const generation = generationRef.current;
            const isCurrent = () => !disposedRef.current && generationRef.current === generation && isStillPressed();
            if (!videoStream.getVideoTracks().length) {
                onError('The camera is not ready yet. Wait for the preview, then hold to record.');
                return;
            }
            let recordStream: MediaStream = videoStream;
            let mirroredSource: MirroredCanvasSource | null = null;
            if (mirror && videoElement) {
                mirroredSource = createMirroredCanvasSource(videoElement);
                if (mirroredSource) recordStream = mirroredSource.stream;
            }
            let audioStream: MediaStream | null = null;
            let released = false;
            const cleanup = () => {
                if (released) return;
                released = true;
                mirroredSource?.stop();
                audioStream?.getTracks().forEach((track) => track.stop());
                if (cleanupRef.current === cleanup) cleanupRef.current = null;
            };
            cleanupRef.current = cleanup;
            try {
                const audio = await navigator.mediaDevices.getUserMedia({ audio: true });
                if (!isCurrent()) {
                    audio.getTracks().forEach((track) => track.stop());
                    cleanup();
                    return;
                }
                audioStream = audio;
                recordStream = new MediaStream([
                    ...(mirroredSource?.stream ?? videoStream).getVideoTracks(),
                    ...audio.getAudioTracks(),
                ]);
            } catch {
                if (!isCurrent()) {
                    cleanup();
                    return;
                }
            }
            startRecording(recordStream, onComplete, cleanup);
        },
        [onError, releaseOwnedResources, startRecording],
    );

    const discardAll = useCallback(() => {
        releaseOwnedResources();
        discardRecording();
    }, [discardRecording, releaseOwnedResources]);

    useEffect(() => {
        disposedRef.current = false;
        return () => {
            disposedRef.current = true;
            releaseOwnedResources();
        };
    }, [releaseOwnedResources]);

    return { recordedVideo, recording, startHeldRecording, stopRecording, discardRecording: discardAll };
}
