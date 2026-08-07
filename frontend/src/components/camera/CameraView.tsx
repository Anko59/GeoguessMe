import React from 'react';
import { CameraErrorPanel, CameraOptionsMenu, CameraTopControls, PreviewActions } from './CameraPanels';
import FilterPicker from './FilterPicker';
import TextBannerEditor, { TextBannerOverlay } from './TextBannerEditor';
import type { Group } from '../../types';
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
    capturedVideo: string | null;
    recording: boolean;
    fileMode: boolean;
    error: string;
    hasMultipleCameras: boolean;
    facingMode: 'user' | 'environment';
    showFilters: boolean;
    showOptions: boolean;
    optionsGroups: Group[];
    selectedGroupIDs: string[];
    hideLocation: boolean;
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
    onToggleOptions: () => void;
    onToggleGroup: (id: string) => void;
    onToggleHideLocation: () => void;
    onCloseOptions: () => void;
    onSelectLens: (lens: LensId) => void;
    onBannerChange: (banner: TextBanner) => void;
    onCaptureButtonClick: () => void;
    onCaptureButtonPointerDown: (event: React.PointerEvent<HTMLButtonElement>) => void;
    onCaptureButtonPointerUp: () => void;
    onCaptureButtonPointerCancel: () => void;
    onFileSelected: (event: React.ChangeEvent<HTMLInputElement>) => void;
    onUpload: () => void;
    onRetake: () => void;
    onCapturedVideoError: () => void;
}

export default function CameraView({
    videoRef,
    overlayCanvasRef,
    captureCanvasRef,
    sourceCanvasRef,
    fileInputRef,
    cameraReady,
    capturedPhoto,
    capturedVideo,
    recording,
    fileMode,
    error,
    hasMultipleCameras,
    facingMode,
    showFilters,
    showOptions,
    optionsGroups,
    selectedGroupIDs,
    hideLocation,
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
    onToggleOptions,
    onToggleGroup,
    onToggleHideLocation,
    onCloseOptions,
    onSelectLens,
    onBannerChange,
    onCaptureButtonClick,
    onCaptureButtonPointerDown,
    onCaptureButtonPointerUp,
    onCaptureButtonPointerCancel,
    onFileSelected,
    onUpload,
    onRetake,
    onCapturedVideoError,
}: CameraViewProps) {
    const mirrorLivePreview = facingMode === 'user';
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
    const optionsButton = (
        <button
            type="button"
            className={`options-toggle-btn${showOptions ? ' active' : ''}`}
            onClick={onToggleOptions}
            aria-label="Challenge options"
            aria-expanded={showOptions}
            title="Challenge options"
        >
            <span aria-hidden="true">⚙️</span>
        </button>
    );
    const optionsMenu = (
        <CameraOptionsMenu
            groups={optionsGroups}
            selectedGroupIDs={selectedGroupIDs}
            hideLocation={hideLocation}
            onToggleGroup={onToggleGroup}
            onToggleHideLocation={onToggleHideLocation}
            onClose={onCloseOptions}
        />
    );

    return (
        <div className="camera-container">
            <video
                ref={videoRef}
                autoPlay
                playsInline
                muted
                className={`camera-video${mirrorLivePreview ? ' mirrored' : ''}${cameraReady && !capturedPhoto && !capturedVideo && !fileMode ? ' ready' : ''}`}
            />
            <canvas ref={captureCanvasRef} className="camera-capture-canvas" aria-hidden="true" />
            <canvas ref={sourceCanvasRef} className="camera-source-canvas" aria-hidden="true" />
            <CameraErrorPanel
                error={error}
                hasPhoto={Boolean(capturedPhoto || capturedVideo)}
                onRetry={onStartCamera}
                onUseFile={onSetFileMode}
            />
            {!capturedPhoto && !capturedVideo ? (
                <div className="camera-view">
                    <canvas
                        ref={overlayCanvasRef}
                        className={`camera-filter-overlay${mirrorLivePreview ? ' mirrored' : ''}`}
                        aria-hidden="true"
                    />
                    <TextBannerOverlay banner={textBanner} />
                    {cameraReady && !fileMode && (
                        <div className="camera-controls">
                            {!recording && (
                                <CameraTopControls
                                    hasMultipleCameras={hasMultipleCameras}
                                    facingMode={facingMode}
                                    showFilters={showFilters}
                                    onSwitchCamera={onSwitchCamera}
                                    onToggleFilters={onToggleFilters}
                                />
                            )}
                            {showFilters && filterPicker}
                            <button
                                className={`capture-button${recording ? ' recording' : ''}`}
                                onClick={onCaptureButtonClick}
                                onPointerDown={onCaptureButtonPointerDown}
                                onPointerUp={onCaptureButtonPointerUp}
                                onPointerCancel={onCaptureButtonPointerCancel}
                                aria-label="Take photo"
                                title={recording ? 'Recording video' : 'Hold to record video'}
                            >
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
                    {capturedPhoto ? (
                        <>
                            <img src={capturedPhoto} alt="Captured" className="preview-image" />
                            <canvas ref={overlayCanvasRef} className="photo-filter-overlay" aria-hidden="true" />
                            <TextBannerOverlay banner={textBanner} />
                        </>
                    ) : (
                        <video
                            src={capturedVideo ?? undefined}
                            className="preview-image"
                            controls
                            playsInline
                            aria-label="Recorded video preview"
                            onError={onCapturedVideoError}
                        />
                    )}
                    <div className="preview-composer">
                        <div className="preview-action-row">
                            {textEditor}
                            {optionsButton}
                        </div>
                        <span className="camera-options-summary">
                            {selectedGroupIDs.length} group{selectedGroupIDs.length === 1 ? '' : 's'}
                            {hideLocation ? ' · location hidden' : ''}
                        </span>
                        {capturedPhoto && fileMode && filterPicker}
                        {showOptions && <div className="camera-options-popover">{optionsMenu}</div>}
                    </div>
                    <PreviewActions uploading={uploading} onRetake={onRetake} onSend={onUpload} />
                </div>
            )}
        </div>
    );
}
