import { useCallback, useEffect, useRef, useState } from 'react';
import { getAPIErrorMessage } from '../../api';
import { dataURLToBlob, fitDimensions, isFilterableImageType, uploadPhoto } from './cameraUtils';
import './Camera.css';
import { CameraErrorPanel, PreviewActions } from './CameraPanels';
import FilterPicker from './FilterPicker';
import TextBannerEditor, { TextBannerOverlay } from './TextBannerEditor';
import type { FaceFrame } from './lenses/facePose';
import type { FaceTracker as FaceTrackerInstance } from './lenses/faceTracker';
import type { LensRenderer as LensRendererInstance } from './lenses/LensRenderer';
import type { LensId } from './lenses/lensCatalog';
import { drawTextBanner, EMPTY_TEXT_BANNER, type TextBanner } from './textBanner';

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
        trackerRef.current?.close();
        rendererRef.current?.dispose();
        trackerRef.current = null;
        rendererRef.current = null;
        setFilterReady(false);
        clearEffects();
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
                renderer.render(frame);
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

    const startCamera = useCallback(async () => {
        const attempt = ++cameraAttemptRef.current;
        filePreparationAttemptRef.current += 1;
        preparedFileDataRef.current = null;
        setFileMode(false);
        setCapturedPhoto(null);
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
            return;
        }
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                video: {
                    facingMode: 'user',
                    width: { ideal: 1280 },
                    height: { ideal: 720 },
                    frameRate: { ideal: 30, max: 30 },
                },
                audio: false,
            });
            if (attempt !== cameraAttemptRef.current) {
                mediaStream.getTracks().forEach((track) => track.stop());
                return;
            }
            streamRef.current = mediaStream;
            const video = videoRef.current;
            if (!video) return;
            video.srcObject = mediaStream;
            const markCameraReady = () => {
                if (attempt !== cameraAttemptRef.current || video.videoWidth === 0 || video.readyState < 2) return;
                setCameraReady(true);
                if (initializedCameraAttemptRef.current === attempt) return;
                initializedCameraAttemptRef.current = attempt;
                if (selectedFilterRef.current !== 'none') {
                    void initializeVideoEffects(video, video.videoWidth, video.videoHeight);
                }
            };
            video.onloadedmetadata = markCameraReady;
            video.onloadeddata = markCameraReady;
            video.oncanplay = markCameraReady;
            void video
                .play()
                .then(markCameraReady)
                .catch(() => undefined);
        } catch (requestError: unknown) {
            if (attempt !== cameraAttemptRef.current) return;
            const name = requestError instanceof DOMException ? requestError.name : '';
            if (name === 'NotAllowedError' || name === 'SecurityError') {
                setError('Camera access denied. Allow camera permissions and try again.');
            } else if (name === 'NotFoundError' || name === 'DevicesNotFoundError') {
                setError('No camera was found. Connect a camera or upload a photo from your device.');
            } else if (name === 'NotReadableError' || name === 'TrackStartError') {
                setError('The camera is busy or unavailable. Close other camera apps and try again.');
            } else {
                setError('The camera could not be started. Try again or upload a photo from your device.');
            }
        }
    }, [destroyEffects, initializeVideoEffects, stopCamera]);

    useEffect(() => {
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
    }, [destroyEffects, startCamera, stopCamera]);

    const capturePhoto = () => {
        const video = videoRef.current;
        const overlay = overlayCanvasRef.current;
        const captureCanvas = captureCanvasRef.current;
        if (!video || !overlay || !captureCanvas || video.videoWidth === 0) return;
        const context = captureCanvas.getContext('2d');
        if (!context) return;
        rendererRef.current?.render(lastFrameRef.current);
        captureCanvas.width = video.videoWidth;
        captureCanvas.height = video.videoHeight;
        context.drawImage(video, 0, 0, captureCanvas.width, captureCanvas.height);
        context.drawImage(overlay, 0, 0, captureCanvas.width, captureCanvas.height);
        const sourceCanvas = sourceCanvasRef.current;
        const sourceContext = sourceCanvas?.getContext('2d');
        if (sourceCanvas && sourceContext) {
            sourceCanvas.width = captureCanvas.width;
            sourceCanvas.height = captureCanvas.height;
            sourceContext.drawImage(captureCanvas, 0, 0);
        }
        const flash = document.createElement('div');
        flash.className = 'camera-flash';
        document.body.appendChild(flash);
        setTimeout(() => flash.remove(), 300);
        setCapturedPhoto(captureCanvas.toDataURL('image/jpeg', 0.9));
        destroyEffects();
        stopCamera();
    };

    const retake = () => {
        setCapturedPhoto(null);
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

    const prepareImageFilter = async (dataURL: string) => {
        const preparationAttempt = filePreparationAttemptRef.current;
        const image = new Image();
        image.onload = async () => {
            if (preparationAttempt !== filePreparationAttemptRef.current) return;
            const sourceCanvas = sourceCanvasRef.current;
            if (!sourceCanvas) return;
            const dimensions = fitDimensions(image.naturalWidth, image.naturalHeight);
            sourceCanvas.width = dimensions.width;
            sourceCanvas.height = dimensions.height;
            const context = sourceCanvas.getContext('2d');
            if (!context) return;
            context.drawImage(image, 0, 0, dimensions.width, dimensions.height);
            preparedFileDataRef.current = dataURL;
            await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
            if (preparationAttempt !== filePreparationAttemptRef.current) return;
            if (selectedFilterRef.current !== 'none') {
                await initializeImageEffects(sourceCanvas, dimensions.width, dimensions.height);
            }
        };
        image.onerror = () => {
            if (preparationAttempt !== filePreparationAttemptRef.current) return;
            preparedFileDataRef.current = null;
            setError('Failed to read the selected file.');
        };
        image.src = dataURL;
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
        reader.onerror = () => setError('Failed to read the selected file.');
        reader.readAsDataURL(file);
    };

    const renderFinalPhoto = (): string | null => {
        const sourceCanvas = sourceCanvasRef.current;
        const captureCanvas = captureCanvasRef.current;
        if (!sourceCanvas || !captureCanvas || sourceCanvas.width === 0) return capturedPhoto;
        const context = captureCanvas.getContext('2d');
        if (!context) return capturedPhoto;
        captureCanvas.width = sourceCanvas.width;
        captureCanvas.height = sourceCanvas.height;
        context.drawImage(sourceCanvas, 0, 0);
        if (fileMode && preparedFileDataRef.current === capturedPhoto) {
            const overlay = overlayCanvasRef.current;
            rendererRef.current?.render(lastFrameRef.current);
            if (overlay) context.drawImage(overlay, 0, 0, sourceCanvas.width, sourceCanvas.height);
        }
        drawTextBanner(context, captureCanvas.width, captureCanvas.height, textBanner);
        return captureCanvas.toDataURL('image/jpeg', 0.9);
    };

    const handleUpload = async () => {
        const photo = renderFinalPhoto();
        if (!photo) return;
        setUploading(true);
        setError('');
        try {
            await uploadPhoto(dataURLToBlob(photo), fileMode ? 'upload.jpg' : 'capture.jpg', groupID);
            destroyEffects();
            stopCamera();
            setCapturedPhoto(null);
            setFileMode(false);
            onUploadComplete();
        } catch (requestError: unknown) {
            const message = requestError instanceof Error ? requestError.message : String(requestError);
            setError(
                /location|geolocation|denied/i.test(message)
                    ? 'Unable to retrieve location. Please enable location services.'
                    : getAPIErrorMessage(requestError, 'Upload failed. Please try again.'),
            );
        } finally {
            setUploading(false);
        }
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

    const filterPicker = (
        <FilterPicker
            selectedFilter={selectedFilter}
            filterReady={filterReady}
            filterError={filterError}
            faceDetected={faceDetected}
            onSelect={selectLens}
        />
    );
    const textEditor = <TextBannerEditor banner={textBanner} onChange={setTextBanner} />;

    return (
        <div className="camera-container">
            <video
                ref={videoRef}
                autoPlay
                playsInline
                muted
                className={`camera-video ${cameraReady && !capturedPhoto && !fileMode ? 'ready' : ''}`}
            />
            <canvas ref={captureCanvasRef} className="camera-capture-canvas" aria-hidden="true" />
            <canvas ref={sourceCanvasRef} className="camera-source-canvas" aria-hidden="true" />
            <CameraErrorPanel
                error={error}
                hasPhoto={Boolean(capturedPhoto)}
                onRetry={() => void startCamera()}
                onUseFile={() => setFileMode(true)}
            />
            {!capturedPhoto ? (
                <div className="camera-view">
                    <canvas ref={overlayCanvasRef} className="camera-filter-overlay" aria-hidden="true" />
                    <TextBannerOverlay banner={textBanner} />
                    {cameraReady && !fileMode && (
                        <div className="camera-controls">
                            {textEditor}
                            {filterPicker}
                            <button className="capture-button" onClick={capturePhoto} aria-label="Take photo">
                                <div className="capture-inner"></div>
                            </button>
                        </div>
                    )}
                    {!cameraReady && !error && !fileMode && (
                        <div className="camera-loading">
                            <div className="spinner"></div>
                            <p>Loading camera...</p>
                        </div>
                    )}
                    {fileMode && (
                        <div className="camera-file-fallback">
                            <label className="btn btn-outline file-fallback-label" htmlFor="camera-file-input">
                                Choose photo from device
                            </label>
                            <input
                                id="camera-file-input"
                                ref={fileInputRef}
                                type="file"
                                accept="image/*"
                                className="camera-file-input"
                                onChange={handleFileSelected}
                            />
                        </div>
                    )}
                </div>
            ) : (
                <div className="photo-preview">
                    <img src={capturedPhoto} alt="Captured" className="preview-image" />
                    <canvas ref={overlayCanvasRef} className="photo-filter-overlay" aria-hidden="true" />
                    <TextBannerOverlay banner={textBanner} />
                    <div className="preview-composer">
                        {textEditor}
                        {fileMode && filterPicker}
                    </div>
                    <PreviewActions uploading={uploading} onRetake={retake} onSend={() => void handleUpload()} />
                </div>
            )}
        </div>
    );
}
