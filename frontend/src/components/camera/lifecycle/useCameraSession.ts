import { useCallback, useEffect, useRef, useState } from 'react';
import { useCameraDevice } from '../cameraUtils';

interface CameraSessionOptions {
    /** Reset capture/file/filter UI state before (re)starting the camera. */
    onReset: () => void;
    /** Called once per camera attempt once the video can play. */
    onReady: (video: HTMLVideoElement, width: number, height: number) => void;
    setError: (message: string) => void;
}

const CAMERA_CONSTRAINTS: MediaStreamConstraints = {
    video: {
        facingMode: 'user',
        width: { ideal: 1280 },
        height: { ideal: 720 },
        frameRate: { ideal: 30, max: 30 },
    },
    audio: false,
};

/** Owns the browser camera lifecycle: permission request, stream creation,
 *  device changes, readiness signalling, restarts, and track cleanup. Every
 *  getUserMedia call is generation-guarded so a stale request can never
 *  attach its stream after a restart or unmount. */
export function useCameraSession({ onReset, onReady, setError }: CameraSessionOptions) {
    const videoRef = useRef<HTMLVideoElement>(null);
    const streamRef = useRef<MediaStream | null>(null);
    const cameraAttemptRef = useRef(0);
    const initializedCameraAttemptRef = useRef(0);
    const [cameraReady, setCameraReady] = useState(false);
    const { facingMode, hasMultipleCameras, facingModeRef, refresh, switchCamera, setRestart } = useCameraDevice();

    const stopCamera = useCallback(() => {
        streamRef.current?.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
    }, []);

    const startCamera = useCallback(async (): Promise<MediaStream | null> => {
        const attempt = ++cameraAttemptRef.current;
        setCameraReady(false);
        setError('');
        stopCamera();
        onReset();
        if (!navigator.mediaDevices?.getUserMedia) {
            setError(
                'Camera access denied or unavailable. Enable camera permissions or upload a photo from your device.',
            );
            return null;
        }
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                ...CAMERA_CONSTRAINTS,
                video: { ...(CAMERA_CONSTRAINTS.video as object), facingMode: facingModeRef.current },
            } as MediaStreamConstraints);

            if (attempt !== cameraAttemptRef.current) {
                mediaStream.getTracks().forEach((track) => track.stop());
                return null;
            }
            streamRef.current = mediaStream;
            void refresh(); // iOS Safari reveals additional cameras after permission.
            const video = videoRef.current;
            if (!video) {
                mediaStream.getTracks().forEach((track) => track.stop());
                streamRef.current = null;
                return null;
            }
            video.srcObject = mediaStream;
            const markCameraReady = () => {
                if (attempt !== cameraAttemptRef.current || video.videoWidth === 0 || video.readyState < 2) return;
                setCameraReady(true);
                if (initializedCameraAttemptRef.current === attempt) return;
                initializedCameraAttemptRef.current = attempt;
                onReady(video, video.videoWidth, video.videoHeight);
            };
            video.onloadedmetadata = markCameraReady;
            video.onloadeddata = markCameraReady;
            video.oncanplay = markCameraReady;
            void video
                .play()
                .then(markCameraReady)
                .catch(() => undefined);
            return mediaStream;
        } catch (requestError: unknown) {
            if (attempt !== cameraAttemptRef.current) return null;
            const name = requestError instanceof DOMException ? requestError.name : '';
            if (name === 'NotAllowedError' || name === 'SecurityError')
                setError('Camera access denied. Allow camera permissions and try again.');
            else if (name === 'NotFoundError' || name === 'DevicesNotFoundError')
                setError('No camera was found. Connect a camera or upload a photo from your device.');
            else if (name === 'NotReadableError' || name === 'TrackStartError')
                setError('The camera is busy or unavailable. Close other camera apps and try again.');
            else setError('The camera could not be started. Try again or upload a photo from your device.');
            return null;
        }
    }, [facingModeRef, onReady, onReset, refresh, setError, stopCamera]);

    // Start on mount, register the restart hook used by camera switching, and
    // invalidate any in-flight attempt on unmount so stale streams are stopped.
    useEffect(() => {
        setRestart(() => startCamera().then(() => undefined));
        let active = true;
        queueMicrotask(() => {
            if (active) void startCamera();
        });
        return () => {
            active = false;
            cameraAttemptRef.current += 1;
            stopCamera();
        };
    }, [setRestart, startCamera, stopCamera]);

    return {
        videoRef,
        streamRef,
        cameraReady,
        startCamera,
        stopCamera,
        facingMode,
        hasMultipleCameras,
        switchCamera,
    };
}
