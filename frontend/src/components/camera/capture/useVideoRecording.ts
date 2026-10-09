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
    const cancelRef = useRef<(() => void) | null>(null);
    const disposedRef = useRef(false);
    const recordedURLRef = useRef<string | null>(null);

    const discardRecording = useCallback(() => {
        if (recordedURLRef.current) URL.revokeObjectURL(recordedURLRef.current);
        recordedURLRef.current = null;
        if (!disposedRef.current) setRecordedVideo(null);
    }, []);

    const stopRecording = useCallback(() => {
        if (recorderRef.current?.state === 'recording') recorderRef.current.stop();
    }, []);

    const startRecording = useCallback(
        (stream: MediaStream, onComplete: () => void, onSettled: () => void = () => {}): boolean => {
            cancelRef.current?.();
            let settled = false;
            let recorder: MediaRecorder | null = null;
            const settle = () => {
                if (settled) return;
                settled = true;
                if (recorderRef.current === recorder) {
                    recorderRef.current = null;
                    cancelRef.current = null;
                    if (!disposedRef.current) setRecording(false);
                }
                onSettled();
            };
            const fail = (message: string) => {
                settle();
                if (!disposedRef.current) onError(message);
                return false;
            };
            if (disposedRef.current) {
                settle();
                return false;
            }
            if (typeof MediaRecorder === 'undefined') {
                return fail('Video recording is not supported by this browser.');
            }
            const mimeType = preferredVideoMIMEType();
            if (!mimeType) return fail('This browser cannot record a compatible video.');
            discardRecording();
            const chunks: BlobPart[] = [];
            let bytes = 0;
            let tooLarge = false;
            let emittedMIMEType = '';
            try {
                recorder = new MediaRecorder(stream, { mimeType });
            } catch {
                return fail('Video recording could not start. Try again.');
            }
            const activeRecorder = recorder;
            const isCurrent = () => !disposedRef.current && !settled && recorderRef.current === activeRecorder;
            recorderRef.current = activeRecorder;
            cancelRef.current = () => {
                // Invalidate before stop: browsers dispatch the final events asynchronously.
                settle();
                if (activeRecorder.state === 'recording') activeRecorder.stop();
            };
            activeRecorder.ondataavailable = (event) => {
                if (!isCurrent() || !event.data.size || tooLarge) return;
                if (!emittedMIMEType && event.data.type) emittedMIMEType = event.data.type;
                bytes += event.data.size;
                if (bytes > MAX_VIDEO_BYTES) {
                    tooLarge = true;
                    if (activeRecorder.state === 'recording') activeRecorder.stop();
                    return;
                }
                chunks.push(event.data);
            };
            activeRecorder.onerror = () => {
                if (!isCurrent()) return;
                fail('Video recording stopped unexpectedly. Please try again.');
                if (activeRecorder.state === 'recording') activeRecorder.stop();
            };
            activeRecorder.onstop = () => {
                if (!isCurrent()) return;
                try {
                    if (tooLarge) {
                        onError('That video is too large. Record a shorter clip (maximum 10 MiB).');
                        return;
                    }
                    const outputMIMEType = emittedMIMEType || activeRecorder.mimeType || mimeType;
                    const blob = new Blob(chunks, { type: outputMIMEType });
                    if (!blob.size) {
                        onError('No video was recorded. Please try again.');
                        return;
                    }
                    const url = URL.createObjectURL(blob);
                    recordedURLRef.current = url;
                    setRecordedVideo({ blob, url });
                    onComplete();
                } finally {
                    settle();
                }
            };
            // One final dataavailable event preserves container initialization metadata.
            try {
                activeRecorder.start();
            } catch {
                fail('Video recording could not start. Try again.');
                if (activeRecorder.state === 'recording') activeRecorder.stop();
                return false;
            }
            setRecording(true);
            return true;
        },
        [discardRecording, onError],
    );

    useEffect(() => {
        disposedRef.current = false;
        return () => {
            disposedRef.current = true;
            cancelRef.current?.();
            if (recordedURLRef.current) URL.revokeObjectURL(recordedURLRef.current);
            recordedURLRef.current = null;
        };
    }, []);

    return { recordedVideo, recording, startRecording, stopRecording, discardRecording };
}
