import { useState, useRef, useEffect, useCallback } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import './Camera.css';

function dataURLToBlob(dataURL: string): Blob {
    const [header, encoded] = dataURL.split(',', 2);
    const binary = atob(encoded);
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    const mimeType = header.match(/^data:([^;]+)/)?.[1] ?? 'image/jpeg';
    return new Blob([bytes], { type: mimeType });
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

    const videoRef = useRef<HTMLVideoElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const streamRef = useRef<MediaStream | null>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);

    const stopCamera = useCallback(() => {
        streamRef.current?.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
    }, []);

    const startCamera = useCallback(async () => {
        setFileMode(false);
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                video: { facingMode: 'environment', width: { ideal: 1920 }, height: { ideal: 1080 } },
                audio: false,
            });
            streamRef.current = mediaStream;
            if (videoRef.current) {
                const video = videoRef.current;
                const markCameraReady = () => setCameraReady(true);
                video.onloadedmetadata = markCameraReady;
                video.oncanplay = markCameraReady;
                video.srcObject = mediaStream;
                void video
                    .play()
                    .then(markCameraReady)
                    .catch(() => undefined);
            }
            setError('');
        } catch {
            setError('Camera access denied. Please allow camera permissions.');
        }
    }, []);

    useEffect(() => {
        void startCamera();
        return stopCamera;
    }, [startCamera, stopCamera]);

    const capturePhoto = () => {
        if (!videoRef.current || !canvasRef.current) return;

        const video = videoRef.current;
        const canvas = canvasRef.current;
        const context = canvas.getContext('2d');

        if (!context) return;

        canvas.width = video.videoWidth;
        canvas.height = video.videoHeight;
        context.drawImage(video, 0, 0);

        // Flash effect
        const flashDiv = document.createElement('div');
        flashDiv.className = 'camera-flash';
        document.body.appendChild(flashDiv);
        setTimeout(() => flashDiv.remove(), 300);

        const photoData = canvas.toDataURL('image/jpeg', 0.8);
        setCapturedPhoto(photoData);
    };

    const retake = () => {
        setCapturedPhoto(null);
        if (fileMode) {
            if (fileInputRef.current) fileInputRef.current.value = '';
        } else {
            startCamera();
        }
    };

    const handleFileSelected = (event: React.ChangeEvent<HTMLInputElement>) => {
        const file = event.target.files?.[0];
        if (!file) return;
        const reader = new FileReader();
        reader.onload = () => {
            if (typeof reader.result === 'string') {
                setCapturedPhoto(reader.result);
                setError('');
            }
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

    const handleUpload = async () => {
        if (!capturedPhoto) return;
        setUploading(true);
        setError('');

        try {
            if (capturedPhoto.startsWith('data:')) {
                const blob = dataURLToBlob(capturedPhoto);
                const filename = fileMode ? 'upload' : 'capture.jpg';
                await uploadBlob(blob, filename);
            } else {
                // File directly from input — use fetch to get blob.
                const response = await fetch(capturedPhoto);
                const blob = await response.blob();
                await uploadBlob(blob, 'upload');
            }
            setCapturedPhoto(null);
            setFileMode(false);
            onUploadComplete();
        } catch (requestError: unknown) {
            const msg = requestError instanceof Error ? requestError.message : String(requestError);
            if (msg.includes('location') || msg.includes('Geolocation') || msg.includes('denied')) {
                setError('Unable to retrieve location. Please enable location services.');
            } else {
                setError(getAPIErrorMessage(requestError, 'Upload failed. Please try again.'));
            }
        } finally {
            setUploading(false);
        }
    };

    return (
        <div className="camera-container">
            {error && (
                <div className="camera-error">
                    <p>{error}</p>
                    {error.includes('Camera') && (
                        <>
                            <button className="btn btn-primary" onClick={startCamera}>
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
                    <video
                        ref={videoRef}
                        autoPlay
                        playsInline
                        muted
                        className={`camera-video ${cameraReady ? 'ready' : ''}`}
                    />
                    {cameraReady && (
                        <div className="camera-controls">
                            <button className="capture-button" onClick={capturePhoto} aria-label="Take photo">
                                <div className="capture-inner"></div>
                            </button>
                        </div>
                    )}
                    {!cameraReady && !error && (
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

            <canvas ref={canvasRef} style={{ display: 'none' }} />
        </div>
    );
}
