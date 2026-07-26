import React from 'react';
import { CameraErrorPanel, CameraTopControls, PreviewActions } from './CameraPanels';
import FilterPicker from './FilterPicker';
import TextBannerEditor, { TextBannerOverlay } from './TextBannerEditor';
import type { LensId } from './lenses/lensCatalog';
import type { TextBanner } from './textBanner';

interface CameraViewProps {
    videoRef: React.RefObject<HTMLVideoElement | null>;
    overlayCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    captureCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    sourceCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    fileInputRef: React.RefObject<HTMLInputElement | null>;
    cameraReady: boolean;
    capturedPhoto: string | null;
    fileMode: boolean;
    error: string;
    hasMultipleCameras: boolean;
    facingMode: 'user' | 'environment';
    showFilters: boolean;
    selectedFilter: LensId;
    filterReady: boolean;
    filterError: string;
    faceDetected: boolean;
    textBanner: TextBanner;
    uploading: boolean;
    onStartCamera: () => void;
    onSetFileMode: () => void;
    onSwitchCamera: () => void;
    onToggleFilters: () => void;
    onSelectLens: (lens: LensId) => void;
    onBannerChange: (banner: TextBanner) => void;
    onCapturePhoto: () => void;
    onFileSelected: (event: React.ChangeEvent<HTMLInputElement>) => void;
    onUpload: () => void;
    onRetake: () => void;
}

export default function CameraView({
    videoRef,
    overlayCanvasRef,
    captureCanvasRef,
    sourceCanvasRef,
    fileInputRef,
    cameraReady,
    capturedPhoto,
    fileMode,
    error,
    hasMultipleCameras,
    facingMode,
    showFilters,
    selectedFilter,
    filterReady,
    filterError,
    faceDetected,
    textBanner,
    uploading,
    onStartCamera,
    onSetFileMode,
    onSwitchCamera,
    onToggleFilters,
    onSelectLens,
    onBannerChange,
    onCapturePhoto,
    onFileSelected,
    onUpload,
    onRetake,
}: CameraViewProps) {
    const filterPicker = (
        <FilterPicker
            selectedFilter={selectedFilter}
            filterReady={filterReady}
            filterError={filterError}
            faceDetected={faceDetected}
            onSelect={onSelectLens}
        />
    );
    const textEditor = <TextBannerEditor banner={textBanner} onChange={onBannerChange} />;

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
                onRetry={onStartCamera}
                onUseFile={onSetFileMode}
            />
            {!capturedPhoto ? (
                <div className="camera-view">
                    <canvas ref={overlayCanvasRef} className="camera-filter-overlay" aria-hidden="true" />
                    <TextBannerOverlay banner={textBanner} />
                    {cameraReady && !fileMode && (
                        <div className="camera-controls">
                            <CameraTopControls
                                hasMultipleCameras={hasMultipleCameras}
                                facingMode={facingMode}
                                showFilters={showFilters}
                                onSwitchCamera={onSwitchCamera}
                                onToggleFilters={onToggleFilters}
                            />
                            {showFilters && filterPicker}
                            {textEditor}
                            <button className="capture-button" onClick={onCapturePhoto} aria-label="Take photo">
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
                                onChange={onFileSelected}
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
                    <PreviewActions uploading={uploading} onRetake={onRetake} onSend={onUpload} />
                </div>
            )}
        </div>
    );
}
