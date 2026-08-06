import { useCallback, useEffect, useRef, useState } from 'react';
import { isFilterableImageType, useCameraDevice } from './cameraUtils';
import { useChallengeOptions, useChallengeUpload } from './useChallengeUpload';
import './Camera.css';
import CameraView from './CameraView';
import { capturePhotoFrame, prepareImageForFilters } from './capture/cameraImagePreparation';
import type { FaceFrame } from './lenses/facePose';
import type { FaceTracker as FaceTrackerInstance } from './lenses/faceTracker';
import type { LensRenderer as LensRendererInstance } from './lenses/LensRenderer';
import type { LensId } from './lenses/lensCatalog';
import { EMPTY_TEXT_BANNER, type TextBanner } from './textBanner';
import { useHoldToRecord } from './capture/useHoldToRecord';
import { useVideoCapture } from './capture/useVideoCapture';
import { useFaceTrackerPreload } from './lenses/useFaceTrackerPreload';
export default function Camera({ groupID, onUploadComplete }: { groupID: string; onUploadComplete: () => void }) {
    const [capturedPhoto, setCapturedPhoto] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');
    const [cameraReady, setCameraReady] = useState(false);
    const [fileMode, setFileMode] = useState(false);
    const [selectedFilter, setSelectedFilter] = useState<LensId>('none');
    const [filterReady, setFilterReady] = useState(false);
    const [filterError, setFilterError] = useState('');
    const [faceDetected, setFaceDetected] = useState(false);
    const [textBanner, setTextBanner] = useState<TextBanner>(EMPTY_TEXT_BANNER);
    const [showFilters, setShowFilters] = useState(
        () => !window.matchMedia('(pointer: coarse), (max-width: 40rem)').matches,
    );
    const {
        showOptions,
        availableGroups,
        targetGroupIDs,
        hideLocation,
        toggleOptions,
        toggleGroup,
        toggleHideLocation,
        closeOptions,
    } = useChallengeOptions(groupID);
    const videoRef = useRef<HTMLVideoElement>(null);
    const overlayCanvasRef = useRef<HTMLCanvasElement>(null);
    const captureCanvasRef = useRef<HTMLCanvasElement>(null);
    const sourceCanvasRef = useRef<HTMLCanvasElement>(null);
    const streamRef = useRef<MediaStream | null>(null);
    const trackerRef = useRef<FaceTrackerInstance | null>(null);
    const rendererRef = useRef<LensRendererInstance | null>(null);
    const trackingAnimationRef = useRef<number | null>(null);
    const lastFrameRef = useRef<FaceFrame | null>(null);
    const selectedFilterRef = useRef<LensId>('none');
    const faceDetectedRef = useRef(false);
    const preparedFileDataRef = useRef<string | null>(null);
    const filePreparationAttemptRef = useRef(0);
    const effectAttemptRef = useRef(0);
    const cameraAttemptRef = useRef(0);
    const initializedCameraAttemptRef = useRef(0);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const locationRequestRef = useRef<Promise<GeolocationPosition> | null>(null);
    const { facingMode, hasMultipleCameras, facingModeRef, refresh, switchCamera, setRestart } = useCameraDevice();
    const recordingError = useCallback((message: string) => setError(message), []);
    const capturedVideoError = useCallback(
        () => setError('The recorded video could not be played. Please record a new clip and try again.'),
        [],
    );
    const { recordedVideo, recording, startHeldRecording, stopRecording, discardRecording } = useVideoCapture({
        onError: recordingError,
    });
    useFaceTrackerPreload();

    const updateFaceDetected = useCallback((detected: boolean) => {
        if (faceDetectedRef.current === detected) return;
        faceDetectedRef.current = detected;
        setFaceDetected(detected);
    }, []);
    const clearEffects = useCallback(() => {
        lastFrameRef.current = null;
        rendererRef.current?.clear();
        updateFaceDetected(false);
    }, [updateFaceDetected]);
    const destroyEffects = useCallback(() => {
        effectAttemptRef.current += 1;
        if (trackingAnimationRef.current !== null) cancelAnimationFrame(trackingAnimationRef.current);
        trackingAnimationRef.current = null;
        clearEffects();
        trackerRef.current?.close();
        rendererRef.current?.dispose();
        trackerRef.current = null;
        rendererRef.current = null;
        setFilterReady(false);
    }, [clearEffects]);
    const createRenderer = useCallback(
        async (
            source: HTMLVideoElement | HTMLCanvasElement,
            width: number,
            height: number,
        ): Promise<LensRendererInstance | null> => {
            const canvas = overlayCanvasRef.current;
            if (!canvas) return null;
            try {
                const { LensRenderer } = await import('./lenses/LensRenderer');
                const renderer = new LensRenderer(canvas);
                renderer.setSource(source);
                renderer.resize(width, height);
                renderer.setLens(selectedFilterRef.current);
                rendererRef.current = renderer;
                return renderer;
            } catch {
                setFilterError('Camera effects require WebGL. Photos can still be sent without a lens.');
                return null;
            }
        },
        [],
    );
    const initializeVideoEffects = useCallback(
        async (video: HTMLVideoElement, width: number, height: number) => {
            const attempt = ++effectAttemptRef.current;
            setFilterReady(false);
            setFilterError('');
            const renderer = await createRenderer(video, width, height);
            if (!renderer) return;
            if (attempt !== effectAttemptRef.current) {
                renderer.dispose();
                if (rendererRef.current === renderer) rendererRef.current = null;
                return;
            }
            try {
                const { FaceTracker } = await import('./lenses/faceTracker');
                const tracker = await FaceTracker.create();
                if (attempt !== effectAttemptRef.current) {
                    tracker.close();
                    return;
                }
                trackerRef.current = tracker;
                setFilterReady(true);
                let lastVideoTime = -1;
                const track = (timestamp: number) => {
                    if (attempt !== effectAttemptRef.current) return;
                    if (video.readyState >= 2 && video.currentTime !== lastVideoTime) {
                        lastVideoTime = video.currentTime;
                        try {
                            const frame = tracker.detectVideo(video, timestamp);
                            lastFrameRef.current = frame;
                            updateFaceDetected(Boolean(frame));
                            renderer.render(frame, timestamp);
                        } catch {
                            setFilterError('Face tracking stopped unexpectedly. Try reopening the camera.');
                            updateFaceDetected(false);
                            return;
                        }
                    } else {
                        renderer.render(lastFrameRef.current, timestamp);
                    }
                    trackingAnimationRef.current = requestAnimationFrame(track);
                };
                trackingAnimationRef.current = requestAnimationFrame(track);
            } catch {
                if (attempt === effectAttemptRef.current) {
                    renderer.dispose();
                    rendererRef.current = null;
                    setFilterError('Face tracking could not start. Photos can still be sent without a lens.');
                }
            }
        },
        [createRenderer, updateFaceDetected],
    );
    const initializeImageEffects = useCallback(
        async (source: HTMLCanvasElement, width: number, height: number) => {
            const attempt = ++effectAttemptRef.current;
            setFilterReady(false);
            setFilterError('');
            const renderer = await createRenderer(source, width, height);
            if (!renderer) return;
            if (attempt !== effectAttemptRef.current) {
                renderer.dispose();
                if (rendererRef.current === renderer) rendererRef.current = null;
                return;
            }
            try {
                const { FaceTracker } = await import('./lenses/faceTracker');
                const tracker = await FaceTracker.create();
                if (attempt !== effectAttemptRef.current) {
                    tracker.close();
                    return;
                }
                trackerRef.current = tracker;
                const frame = await tracker.detectImage(source);
                if (attempt !== effectAttemptRef.current) return;
                lastFrameRef.current = frame;
                updateFaceDetected(Boolean(frame));
                const animate = (timestamp: number) => {
                    if (attempt !== effectAttemptRef.current) return;
                    renderer.render(frame, timestamp);
                    trackingAnimationRef.current = requestAnimationFrame(animate);
                };
                animate(performance.now());
                setFilterReady(true);
                if (!frame) setFilterError('No face found. Try a brighter, front-facing photo.');
            } catch {
                if (attempt === effectAttemptRef.current) {
                    setFilterError('This photo could not be tracked. The original can still be sent.');
                }
            }
        },
        [createRenderer, updateFaceDetected],
    );
    const stopCamera = useCallback(() => {
        streamRef.current?.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
    }, []);
    const startCamera = useCallback(async (): Promise<MediaStream | null> => {
        const attempt = ++cameraAttemptRef.current;
        filePreparationAttemptRef.current += 1;
        preparedFileDataRef.current = null;
        setFileMode(false);
        setCapturedPhoto(null);
        discardRecording();
        setCameraReady(false);
        setError('');
        setFilterError('');
        if (sourceCanvasRef.current) sourceCanvasRef.current.width = 0;
        stopCamera();
        destroyEffects();
        if (!navigator.mediaDevices?.getUserMedia) {
            setError(
                'Camera access denied or unavailable. Enable camera permissions or upload a photo from your device.',
            );
            return null;
        }
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                video: {
                    facingMode: facingModeRef.current,
                    width: { ideal: 1280 },
                    height: { ideal: 720 },
                    frameRate: { ideal: 30, max: 30 },
                },
                audio: false,
            });

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
                if (selectedFilterRef.current !== 'none')
                    void initializeVideoEffects(video, video.videoWidth, video.videoHeight);
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
    }, [destroyEffects, discardRecording, facingModeRef, initializeVideoEffects, refresh, stopCamera]);
    const startHeldVideo = async (isStillPressed: () => boolean) => {
        if (recording) return;
        const videoStream = streamRef.current;
        if (!videoStream) return;
        setError('');
        await startHeldRecording(
            videoStream,
            isStillPressed,
            () => {
                destroyEffects();
                stopCamera();
            },
            facingMode === 'user',
            videoRef.current,
        );
    };
    useEffect(() => {
        setRestart(() => {
            stopCamera();
            destroyEffects();
            return startCamera().then(() => undefined);
        });
        let active = true;
        queueMicrotask(() => {
            if (active) void startCamera();
        });
        return () => {
            active = false;
            cameraAttemptRef.current += 1;
            destroyEffects();
            stopCamera();
        };
    }, [setRestart, destroyEffects, startCamera, stopCamera]);
    const { requestLocation, handleUpload } = useChallengeUpload({
        groupIDs: targetGroupIDs,
        hideLocation,
        fileMode,
        capturedPhoto,
        textBanner,
        recordedVideo,
        sourceCanvasRef,
        captureCanvasRef,
        overlayCanvasRef,
        rendererRef,
        lastFrameRef,
        preparedFileDataRef,
        locationRequestRef,
        destroyEffects,
        stopCamera,
        discardRecording,
        onUploadComplete,
        setCapturedPhoto,
        setFileMode,
        setError,
        setUploading,
    });

    useEffect(() => {
        void requestLocation();
    }, [requestLocation]);
    const capturePhoto = () => {
        const photo = capturePhotoFrame({
            video: videoRef.current,
            overlay: overlayCanvasRef.current,
            captureCanvas: captureCanvasRef.current,
            sourceCanvas: sourceCanvasRef.current,
            renderer: rendererRef.current,
            frame: lastFrameRef.current,
            // Front cameras preview mirrored, so the captured photo must be
            // flipped to match what the user saw; back cameras stay as-is.
            mirror: facingMode === 'user',
        });
        if (!photo) return;
        const flash = document.createElement('div');
        flash.className = 'camera-flash';
        document.body.appendChild(flash);
        setTimeout(() => flash.remove(), 300);
        setCapturedPhoto(photo);
        destroyEffects();
        stopCamera();
    };
    const retake = () => {
        setCapturedPhoto(null);
        discardRecording();
        destroyEffects();
        if (fileMode) {
            filePreparationAttemptRef.current += 1;
            preparedFileDataRef.current = null;
            if (fileInputRef.current) fileInputRef.current.value = '';
            setFilterError('');
        } else {
            void startCamera();
        }
    };
    const prepareImageFilter = (dataURL: string) => {
        const preparationAttempt = filePreparationAttemptRef.current;
        prepareImageForFilters({
            dataURL,
            isCurrent: () => preparationAttempt === filePreparationAttemptRef.current,
            sourceCanvas: sourceCanvasRef.current,
            onPrepared: async (sourceCanvas, width, height) => {
                preparedFileDataRef.current = dataURL;
                if (selectedFilterRef.current !== 'none') await initializeImageEffects(sourceCanvas, width, height);
            },
            onError: () => {
                preparedFileDataRef.current = null;
                setError('Failed to read the selected file.');
            },
        });
    };
    const handleFileSelected = (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;
        filePreparationAttemptRef.current += 1;
        preparedFileDataRef.current = null;
        const canPrepareFilter = isFilterableImageType(file.type);
        setFilterError(
            canPrepareFilter ? '' : '3D lenses support JPEG, PNG, and WebP. The original photo can still be sent.',
        );
        stopCamera();
        destroyEffects();
        const reader = new FileReader();
        reader.onload = () => {
            if (typeof reader.result !== 'string') return;
            setCapturedPhoto(reader.result);
            setFileMode(true);
            setError('');
            if (canPrepareFilter) void prepareImageFilter(reader.result);
        };
        reader.onerror = () => {
            preparedFileDataRef.current = null;
            setError('Failed to read the selected file.');
        };
        reader.readAsDataURL(file);
    };
    const selectLens = (lens: LensId) => {
        selectedFilterRef.current = lens;
        setSelectedFilter(lens);
        if (lens === 'none') {
            destroyEffects();
            return;
        }
        if (rendererRef.current) {
            rendererRef.current.setLens(lens);
            rendererRef.current.render(lastFrameRef.current);
            return;
        }
        const sourceCanvas = sourceCanvasRef.current;
        if (fileMode && sourceCanvas && sourceCanvas.width > 0) {
            void initializeImageEffects(sourceCanvas, sourceCanvas.width, sourceCanvas.height);
            return;
        }
        const video = videoRef.current;
        if (cameraReady && video && video.videoWidth > 0) {
            void initializeVideoEffects(video, video.videoWidth, video.videoHeight);
        }
    };
    const captureGesture = useHoldToRecord({
        onHold: startHeldVideo,
        onStop: stopRecording,
        onTap: capturePhoto,
    });
    // While recording, tapping the capture button stops the clip instead of taking a photo.
    const captureButtonClick = recording ? stopRecording : captureGesture.onClick;

    return (
        <CameraView
            videoRef={videoRef}
            overlayCanvasRef={overlayCanvasRef}
            captureCanvasRef={captureCanvasRef}
            sourceCanvasRef={sourceCanvasRef}
            fileInputRef={fileInputRef}
            cameraReady={cameraReady}
            capturedPhoto={capturedPhoto}
            capturedVideo={recordedVideo?.url ?? null}
            recording={recording}
            fileMode={fileMode}
            error={error}
            hasMultipleCameras={hasMultipleCameras}
            facingMode={facingMode}
            showFilters={showFilters}
            showOptions={showOptions}
            optionsGroups={availableGroups}
            selectedGroupIDs={targetGroupIDs}
            hideLocation={hideLocation}
            selectedFilter={selectedFilter}
            filterReady={filterReady}
            filterError={filterError}
            faceDetected={faceDetected}
            textBanner={textBanner}
            uploading={uploading}
            onStartCamera={() => void startCamera()}
            onSetFileMode={() => setFileMode(true)}
            onSwitchCamera={switchCamera}
            onToggleFilters={() => setShowFilters((p) => !p)}
            onToggleOptions={toggleOptions}
            onToggleGroup={toggleGroup}
            onToggleHideLocation={toggleHideLocation}
            onCloseOptions={closeOptions}
            onSelectLens={selectLens}
            onBannerChange={setTextBanner}
            onCaptureButtonClick={captureButtonClick}
            onCaptureButtonPointerDown={captureGesture.onPointerDown}
            onCaptureButtonPointerUp={captureGesture.onPointerUp}
            onCaptureButtonPointerCancel={captureGesture.onPointerCancel}
            onFileSelected={handleFileSelected}
            onUpload={() => void handleUpload()}
            onRetake={retake}
            onCapturedVideoError={capturedVideoError}
        />
    );
}
