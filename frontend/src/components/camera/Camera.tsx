import { useCallback, useEffect, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import {
    clearFaceFilter,
    drawFaceFilter,
    FACE_FILTER_OPTIONS,
    type FaceDetectionState,
    type FaceFilterId,
} from '../../faceFilters';
import './Camera.css';

const FACE_FILTER_MODEL_PATH = '/vendor/jeeliz/neuralNets/';

function dataURLToBlob(dataURL: string): Blob {
    const [header, encoded] = dataURL.split(',', 2);
    const binary = atob(encoded);
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    const mimeType = header.match(/^data:([^;]+)/)?.[1] ?? 'image/jpeg';
    return new Blob([bytes], { type: mimeType });
}

function fitDimensions(width: number, height: number): { width: number; height: number } {
    const maxDimension = 2048;
    const scale = Math.min(1, maxDimension / Math.max(width, height));
    return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) };
}

interface CameraProps {
    groupID: string;
    onUploadComplete: () => void;
}

export default function Camera({ groupID, onUploadComplete }: CameraProps) {
    const [capturedPhoto, setCapturedPhoto] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');
    const [cameraReady, setCameraReady] = useState(false);
    const [fileMode, setFileMode] = useState(false);
    const [selectedFilter, setSelectedFilter] = useState<FaceFilterId>('none');
    const [filterReady, setFilterReady] = useState(false);
    const [filterError, setFilterError] = useState('');

    const videoRef = useRef<HTMLVideoElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const overlayCanvasRef = useRef<HTMLCanvasElement>(null);
    const captureCanvasRef = useRef<HTMLCanvasElement>(null);
    const sourceCanvasRef = useRef<HTMLCanvasElement>(null);
    const streamRef = useRef<MediaStream | null>(null);
    const faceFilterRef = useRef<JeelizFaceFilterApi | null>(null);
    const filterAttemptRef = useRef(0);
    const cameraAttemptRef = useRef(0);
    const initializedCameraAttemptRef = useRef(0);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const selectedFilterRef = useRef<FaceFilterId>('none');
    const lastDetectionRef = useRef<FaceDetectionState | null>(null);

    const clearOverlay = useCallback(() => {
        const overlay = overlayCanvasRef.current;
        const context = overlay?.getContext('2d');
        if (overlay && context) clearFaceFilter(context, overlay.width, overlay.height);
        lastDetectionRef.current = null;
    }, []);

    const destroyFaceFilter = useCallback(async () => {
        filterAttemptRef.current += 1;
        const faceFilter = faceFilterRef.current;
        faceFilterRef.current = null;
        setFilterReady(false);
        clearOverlay();
        if (!faceFilter) return;
        try {
            await faceFilter.destroy();
        } catch {
            setFilterError('Face filters need to be restarted. Photos can still be sent without a filter.');
        }
    }, [clearOverlay]);

    const drawCurrentFilter = useCallback((state: FaceDetectionState | null) => {
        const overlay = overlayCanvasRef.current;
        if (!overlay) return;
        const context = overlay.getContext('2d');
        if (!context) return;
        lastDetectionRef.current = state;
        drawFaceFilter(context, selectedFilterRef.current, state, overlay.width, overlay.height);
    }, []);

    const initializeFaceFilter = useCallback(
        async (sourceVideo: HTMLVideoElement, width: number, height: number) => {
            const attempt = filterAttemptRef.current + 1;
            filterAttemptRef.current = attempt;
            const filterCanvas = canvasRef.current;
            const overlay = overlayCanvasRef.current;
            const faceFilter = window.JEELIZFACEFILTER;
            if (!filterCanvas || !overlay || !faceFilter) {
                setFilterError(
                    'Face filters are unavailable in this browser. Photos can still be sent without a filter.',
                );
                return;
            }

            filterCanvas.width = width;
            filterCanvas.height = height;
            overlay.width = width;
            overlay.height = height;
            clearOverlay();
            setFilterReady(false);
            setFilterError('');

            try {
                const initialized = faceFilter.init({
                    canvas: filterCanvas,
                    NNCPath: FACE_FILTER_MODEL_PATH,
                    videoSettings: { videoElement: sourceVideo },
                    callbackReady: (errorCode) => {
                        if (attempt !== filterAttemptRef.current) return;
                        if (errorCode) {
                            setFilterError('Face filters could not start. Photos can still be sent without a filter.');
                            return;
                        }
                        faceFilterRef.current = faceFilter;
                        setFilterReady(true);
                    },
                    callbackTrack: (state) => {
                        if (attempt !== filterAttemptRef.current) return;
                        faceFilter.render_video();
                        drawCurrentFilter(state);
                    },
                });
                if (!initialized && attempt === filterAttemptRef.current) {
                    setFilterError('Face filters could not start. Photos can still be sent without a filter.');
                }
            } catch {
                if (attempt === filterAttemptRef.current) {
                    setFilterError('Face filters could not start. Photos can still be sent without a filter.');
                }
            }
        },
        [clearOverlay, drawCurrentFilter],
    );

    const stopCamera = useCallback(() => {
        const stream = streamRef.current;
        if (stream && typeof stream.getTracks === 'function') {
            stream.getTracks().forEach((track) => track.stop());
        }
        streamRef.current = null;
    }, []);

    const startCamera = useCallback(async () => {
        const attempt = ++cameraAttemptRef.current;
        setFileMode(false);
        setCapturedPhoto(null);
        setCameraReady(false);
        setError('');
        setFilterError('');
        stopCamera();
        await destroyFaceFilter();
        if (attempt !== cameraAttemptRef.current) return;
        if (!navigator.mediaDevices?.getUserMedia) {
            setError(
                'Camera access denied or unavailable. Enable camera permissions or upload a photo from your device.',
            );
            return;
        }
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                video: { facingMode: 'environment', width: { ideal: 1920 }, height: { ideal: 1080 } },
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
                if (attempt !== cameraAttemptRef.current || video.videoWidth === 0) return;
                setCameraReady(true);
                if (initializedCameraAttemptRef.current === attempt) return;
                initializedCameraAttemptRef.current = attempt;
                void initializeFaceFilter(video, video.videoWidth, video.videoHeight);
            };
            video.onloadedmetadata = markCameraReady;
            video.oncanplay = markCameraReady;
            void video
                .play()
                .then(markCameraReady)
                .catch(() => undefined);
            setError('');
        } catch (requestError: unknown) {
            if (attempt !== cameraAttemptRef.current) return;
            const name = requestError instanceof DOMException ? requestError.name : '';
            if (name === 'NotAllowedError' || name === 'SecurityError') {
                setError('Camera access denied. Allow camera permissions and try again.');
            } else if (name === 'NotFoundError' || name === 'DevicesNotFoundError') {
                setError('No camera was found. Connect a camera or upload a photo from your device.');
            } else if (name === 'NotReadableError' || name === 'TrackStartError') {
                setError('The camera is busy or unavailable. Close other camera apps and try again.');
            } else if (requestError instanceof Error && /denied|permission/i.test(requestError.message)) {
                setError('Camera access denied. Allow camera permissions and try again.');
            } else {
                setError('The camera could not be started. Try again or upload a photo from your device.');
            }
        }
    }, [destroyFaceFilter, initializeFaceFilter, stopCamera]);

    useEffect(() => {
        let active = true;
        queueMicrotask(() => {
            if (active) void startCamera();
        });
        return () => {
            active = false;
            cameraAttemptRef.current += 1;
            void destroyFaceFilter();
            stopCamera();
        };
    }, [destroyFaceFilter, startCamera, stopCamera]);

    useEffect(() => {
        selectedFilterRef.current = selectedFilter;
        drawCurrentFilter(lastDetectionRef.current);
    }, [drawCurrentFilter, selectedFilter]);

    const capturePhoto = () => {
        const video = videoRef.current;
        const overlay = overlayCanvasRef.current;
        const captureCanvas = captureCanvasRef.current;
        if (!video || !overlay || !captureCanvas || video.videoWidth === 0) return;
        const context = captureCanvas.getContext('2d');
        if (!context) return;

        captureCanvas.width = video.videoWidth;
        captureCanvas.height = video.videoHeight;
        context.drawImage(video, 0, 0, captureCanvas.width, captureCanvas.height);
        context.drawImage(overlay, 0, 0, captureCanvas.width, captureCanvas.height);
        const flashDiv = document.createElement('div');
        flashDiv.className = 'camera-flash';
        document.body.appendChild(flashDiv);
        setTimeout(() => flashDiv.remove(), 300);
        setCapturedPhoto(captureCanvas.toDataURL('image/jpeg', 0.8));
        void destroyFaceFilter();
        stopCamera();
    };

    const retake = () => {
        setCapturedPhoto(null);
        clearOverlay();
        if (fileMode) {
            if (fileInputRef.current) fileInputRef.current.value = '';
            setFilterError('');
        } else {
            void startCamera();
        }
    };

    const prepareImageFilter = async (dataURL: string) => {
        const image = new Image();
        image.onload = async () => {
            const sourceCanvas = sourceCanvasRef.current;
            const sourceVideo = videoRef.current;
            if (!sourceCanvas || !sourceVideo) return;
            const dimensions = fitDimensions(image.naturalWidth, image.naturalHeight);
            sourceCanvas.width = dimensions.width;
            sourceCanvas.height = dimensions.height;
            const context = sourceCanvas.getContext('2d');
            if (!context) return;
            context.drawImage(image, 0, 0, dimensions.width, dimensions.height);
            if (!sourceCanvas.captureStream) {
                setFilterError('Face filters require a modern browser. Photos can still be sent without a filter.');
                return;
            }
            const stream = sourceCanvas.captureStream(1);
            streamRef.current = stream;
            sourceVideo.srcObject = stream;
            await sourceVideo.play().catch(() => undefined);
            await destroyFaceFilter();
            void initializeFaceFilter(sourceVideo, dimensions.width, dimensions.height);
        };
        image.onerror = () => setError('Failed to read the selected file.');
        image.src = dataURL;
    };

    const handleFileSelected = (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;
        stopCamera();
        void destroyFaceFilter();
        const reader = new FileReader();
        reader.onload = () => {
            if (typeof reader.result !== 'string') return;
            setCapturedPhoto(reader.result);
            setFileMode(true);
            setError('');
            void prepareImageFilter(reader.result);
        };
        reader.onerror = () => setError('Failed to read the selected file.');
        reader.readAsDataURL(file);
    };

    const openFilePicker = () => {
        setError('');
        setFileMode(true);
    };

    const uploadBlob = async (blob: Blob, filename: string) => {
        const position = await new Promise<GeolocationPosition>((resolve, reject) => {
            if (!navigator.geolocation) {
                reject(new Error('Geolocation is not supported by your browser'));
                return;
            }
            navigator.geolocation.getCurrentPosition(resolve, reject);
        });
        const formData = new FormData();
        formData.append('photo', blob, filename);
        formData.append('group_id', groupID);
        formData.append('lat', position.coords.latitude.toString());
        formData.append('long', position.coords.longitude.toString());
        await api.post('/photo/upload', formData);
    };

    const renderFilePhoto = (): string | null => {
        const sourceCanvas = sourceCanvasRef.current;
        const overlay = overlayCanvasRef.current;
        const captureCanvas = captureCanvasRef.current;
        if (!sourceCanvas || !overlay || !captureCanvas || sourceCanvas.width === 0) return capturedPhoto;
        const context = captureCanvas.getContext('2d');
        if (!context) return capturedPhoto;
        captureCanvas.width = sourceCanvas.width;
        captureCanvas.height = sourceCanvas.height;
        context.drawImage(sourceCanvas, 0, 0);
        context.drawImage(overlay, 0, 0, sourceCanvas.width, sourceCanvas.height);
        return captureCanvas.toDataURL('image/jpeg', 0.8);
    };

    const handleUpload = async () => {
        const photo = fileMode ? renderFilePhoto() : capturedPhoto;
        if (!photo) return;
        setUploading(true);
        setError('');
        try {
            await uploadBlob(dataURLToBlob(photo), fileMode ? 'upload.jpg' : 'capture.jpg');
            void destroyFaceFilter();
            stopCamera();
            setCapturedPhoto(null);
            setFileMode(false);
            onUploadComplete();
        } catch (requestError: unknown) {
            const msg = requestError instanceof Error ? requestError.message : String(requestError);
            setError(
                msg.includes('location') || msg.includes('Geolocation') || msg.includes('denied')
                    ? 'Unable to retrieve location. Please enable location services.'
                    : getAPIErrorMessage(requestError, 'Upload failed. Please try again.'),
            );
        } finally {
            setUploading(false);
        }
    };

    const filterPicker = (
        <div className="camera-filter-picker" role="group" aria-label="Photo filters">
            <span className="camera-filter-label">Filters</span>
            <div className="camera-filter-options">
                {FACE_FILTER_OPTIONS.map((option) => (
                    <button
                        key={option.id}
                        type="button"
                        className={`camera-filter-option ${selectedFilter === option.id ? 'selected' : ''}`}
                        aria-pressed={selectedFilter === option.id}
                        onClick={() => setSelectedFilter(option.id)}
                    >
                        {option.label}
                    </button>
                ))}
            </div>
            {selectedFilter !== 'none' && !filterReady && !filterError && <small>Loading face tracking…</small>}
            {filterError && <small className="camera-filter-status">{filterError}</small>}
        </div>
    );

    return (
        <div className="camera-container">
            <video
                ref={videoRef}
                autoPlay
                playsInline
                muted
                className={`camera-video ${cameraReady && !capturedPhoto && !fileMode ? 'ready' : ''}`}
            />
            <canvas ref={canvasRef} className="camera-engine-canvas" aria-hidden="true" />
            <canvas ref={captureCanvasRef} className="camera-capture-canvas" aria-hidden="true" />
            <canvas ref={sourceCanvasRef} className="camera-source-canvas" aria-hidden="true" />

            {error && (
                <div className="camera-error">
                    <p>{error}</p>
                    {!capturedPhoto && (
                        <>
                            <button className="btn btn-primary" onClick={() => void startCamera()}>
                                Try Again
                            </button>
                            <button className="btn btn-outline file-fallback-btn" onClick={openFilePicker}>
                                Upload from device
                            </button>
                        </>
                    )}
                </div>
            )}

            {!capturedPhoto ? (
                <div className="camera-view">
                    <canvas ref={overlayCanvasRef} className="camera-filter-overlay" aria-hidden="true" />
                    {cameraReady && !fileMode && (
                        <div className="camera-controls">
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
                    {fileMode && <div className="preview-filter-picker">{filterPicker}</div>}
                    <div className="preview-controls">
                        <button className="btn btn-outline" onClick={retake} disabled={uploading}>
                            Retake
                        </button>
                        <button className="btn btn-primary" onClick={handleUpload} disabled={uploading}>
                            {uploading ? 'Sending...' : 'Send 📸'}
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
