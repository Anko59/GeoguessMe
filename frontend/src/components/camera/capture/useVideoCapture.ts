import { useCallback, useEffect, useRef } from 'react';
import { useVideoRecording, type RecordedVideo } from './useVideoRecording';

export type { RecordedVideo };

interface UseVideoCaptureOptions {
    onError: (message: string) => void;
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
        async (videoStream: MediaStream, isStillPressed: () => boolean, onComplete: () => void) => {
            if (!videoStream.getVideoTracks().length) {
                onError('The camera is not ready yet. Wait for the preview, then hold to record.');
                return;
            }
            let recordStream = videoStream;
            try {
                const audio = await navigator.mediaDevices.getUserMedia({ audio: true });
                if (!isStillPressed()) {
                    audio.getTracks().forEach((track) => track.stop());
                    return;
                }
                audioStreamRef.current = audio;
                recordStream = new MediaStream([...videoStream.getVideoTracks(), ...audio.getAudioTracks()]);
            } catch {
                if (!isStillPressed()) return;
            }
            startRecording(recordStream, () => {
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
