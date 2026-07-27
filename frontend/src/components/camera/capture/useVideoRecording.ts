import { useCallback, useEffect, useRef, useState } from 'react';

const MAX_VIDEO_BYTES = 10 * 1024 * 1024;

export interface RecordedVideo {
    blob: Blob;
    url: string;
}

function preferredVideoMIMEType(): string | undefined {
    if (typeof MediaRecorder === 'undefined') return undefined;
    const candidates = ['video/webm;codecs=vp8,opus', 'video/webm', 'video/mp4'];
    return candidates.find((type) => MediaRecorder.isTypeSupported(type));
}

export function useVideoRecording(onError: (message: string) => void) {
    const [recording, setRecording] = useState(false);
    const [recordedVideo, setRecordedVideo] = useState<RecordedVideo | null>(null);
    const recorderRef = useRef<MediaRecorder | null>(null);
    const recordedURLRef = useRef<string | null>(null);

    const discardRecording = useCallback(() => {
        if (recordedURLRef.current) URL.revokeObjectURL(recordedURLRef.current);
        recordedURLRef.current = null;
        setRecordedVideo(null);
    }, []);

    const stopRecording = useCallback(() => {
        if (recorderRef.current?.state === 'recording') recorderRef.current.stop();
    }, []);

    const startRecording = useCallback(
        (stream: MediaStream, onComplete: () => void): boolean => {
            if (typeof MediaRecorder === 'undefined') {
                onError('Video recording is not supported by this browser.');
                return false;
            }
            const mimeType = preferredVideoMIMEType();
            if (!mimeType) {
                onError('This browser cannot record a compatible video.');
                return false;
            }
            discardRecording();
            const chunks: BlobPart[] = [];
            let bytes = 0;
            let tooLarge = false;
            let recorder: MediaRecorder;
            try {
                recorder = new MediaRecorder(stream, { mimeType });
            } catch {
                onError('Video recording could not start. Try again.');
                return false;
            }
            recorderRef.current = recorder;
            recorder.ondataavailable = (event) => {
                if (!event.data.size) return;
                bytes += event.data.size;
                if (bytes > MAX_VIDEO_BYTES) {
                    tooLarge = true;
                    recorder.stop();
                    return;
                }
                chunks.push(event.data);
            };
            recorder.onerror = () => onError('Video recording stopped unexpectedly. Please try again.');
            recorder.onstop = () => {
                recorderRef.current = null;
                setRecording(false);
                if (tooLarge) {
                    onError('That video is too large. Record a shorter clip (maximum 10 MiB).');
                    return;
                }
                const blob = new Blob(chunks, { type: mimeType });
                if (!blob.size) {
                    onError('No video was recorded. Please try again.');
                    return;
                }
                const url = URL.createObjectURL(blob);
                recordedURLRef.current = url;
                setRecordedVideo({ blob, url });
                onComplete();
            };
            recorder.start(1000);
            setRecording(true);
            return true;
        },
        [discardRecording, onError],
    );

    useEffect(
        () => () => {
            if (recorderRef.current?.state === 'recording') recorderRef.current.stop();
            if (recordedURLRef.current) URL.revokeObjectURL(recordedURLRef.current);
        },
        [],
    );

    return { recordedVideo, recording, startRecording, stopRecording, discardRecording };
}
