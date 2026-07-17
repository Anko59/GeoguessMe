import { useState, useRef, useEffect, useCallback } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import './Camera.css';

interface CameraProps {
    groupID: string;
    onUploadComplete: () => void;
}

export default function Camera({ groupID, onUploadComplete }: CameraProps) {
    const [capturedPhoto, setCapturedPhoto] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');
    const [cameraReady, setCameraReady] = useState(false);

    const videoRef = useRef<HTMLVideoElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const streamRef = useRef<MediaStream | null>(null);

    const stopCamera = useCallback(() => {
        streamRef.current?.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
    }, []);

    const startCamera = useCallback(async () => {
        try {
            const mediaStream = await navigator.mediaDevices.getUserMedia({
                video: { facingMode: 'environment', width: { ideal: 1920 }, height: { ideal: 1080 } },
                audio: false
            });
            streamRef.current = mediaStream;
            if (videoRef.current) {
                videoRef.current.srcObject = mediaStream;
                videoRef.current.onloadedmetadata = () => {
                    setCameraReady(true);
                };
            }
            setError('');
        } catch {
            setError('Camera access denied. Please allow camera permissions.');
        }
    }, []);

    useEffect(() => { void startCamera(); return stopCamera; }, [startCamera, stopCamera]);

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
        startCamera();
    };

    const handleUpload = async () => {
        if (!capturedPhoto) return;
        setUploading(true);
        setError('');

        if (!navigator.geolocation) {
            setError('Geolocation is not supported by your browser');
            setUploading(false);
            return;
        }

        navigator.geolocation.getCurrentPosition(
            async (position) => {
                try {
                    // Convert base64 to blob
                    const response = await fetch(capturedPhoto);
                    const blob = await response.blob();

                    const formData = new FormData();
                    formData.append('photo', blob, 'capture.jpg');
                    formData.append('group_id', groupID);
                    formData.append('lat', position.coords.latitude.toString());
                    formData.append('long', position.coords.longitude.toString());

                    await api.post('/photo/upload', formData);

                    setCapturedPhoto(null);
                    onUploadComplete();
                } catch (requestError: unknown) {
                    setError(getAPIErrorMessage(requestError, 'Upload failed. Please try again.'));
                } finally {
                    setUploading(false);
                }
            },
            () => {
                setError('Unable to retrieve location. Please enable location services.');
                setUploading(false);
            }
        );
    };

    return (
        <div className="camera-container">
            {error && (
                <div className="camera-error">
                    <p>{error}</p>
                    {error.includes('Camera') && (
                        <button className="btn btn-primary" onClick={startCamera}>
                            Try Again
                        </button>
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
                            <button className="capture-button" onClick={capturePhoto}>
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
                </div>
            ) : (
                <div className="photo-preview">
                    <img src={capturedPhoto} alt="Captured" className="preview-image" />
                    <div className="preview-controls">
                        <button
                            className="btn btn-outline"
                            onClick={retake}
                            disabled={uploading}
                        >
                            Retake
                        </button>
                        <button
                            className="btn btn-primary"
                            onClick={handleUpload}
                            disabled={uploading}
                        >
                            {uploading ? 'Sending...' : 'Send 📸'}
                        </button>
                    </div>
                </div>
            )}

            <canvas ref={canvasRef} style={{ display: 'none' }} />
        </div>
    );
}
