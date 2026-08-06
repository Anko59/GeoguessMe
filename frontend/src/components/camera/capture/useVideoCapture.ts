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
    const stream = canvas.captureStream(30);
    let frameID = 0;
    const drawFrame = () => {
        if (context && video.readyState >= 2 && video.videoWidth > 0) {
            context.setTransform(-1, 0, 0, 1, canvas.width, 0);
            context.drawImage(video, 0, 0, canvas.width, canvas.height);
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
    const audioStreamRef = useRef<MediaStream | null>(null);

    const stopAudioStream = useCallback(() => {
        audioStreamRef.current?.getTracks().forEach((track) => track.stop());
        audioStreamRef.current = null;
    }, []);

    const startHeldRecording = useCallback(
        async (
            videoStream: MediaStream,
            isStillPressed: () => boolean,
            onComplete: () => void,
            mirror = false,
            videoElement: HTMLVideoElement | null = null,
        ) => {
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
            try {
                const audio = await navigator.mediaDevices.getUserMedia({ audio: true });
                if (!isStillPressed()) {
                    mirroredSource?.stop();
                    audio.getTracks().forEach((track) => track.stop());
                    return;
                }
                audioStreamRef.current = audio;
                recordStream = new MediaStream([
                    ...(mirroredSource?.stream ?? videoStream).getVideoTracks(),
                    ...audio.getAudioTracks(),
                ]);
            } catch {
                if (!isStillPressed()) {
                    mirroredSource?.stop();
                    return;
                }
            }
            startRecording(recordStream, () => {
                mirroredSource?.stop();
                stopAudioStream();
                onComplete();
            });
        },
        [onError, startRecording, stopAudioStream],
    );

    const discardAll = useCallback(() => {
        stopAudioStream();
        discardRecording();
    }, [discardRecording, stopAudioStream]);

    useEffect(
        () => () => {
            stopAudioStream();
        },
        [stopAudioStream],
    );

    return { recordedVideo, recording, startHeldRecording, stopRecording, discardRecording: discardAll };
}
