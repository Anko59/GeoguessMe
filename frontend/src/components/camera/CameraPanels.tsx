export function CameraTopControls({
    hasMultipleCameras,
    facingMode,
    showFilters,
    onSwitchCamera,
    onToggleFilters,
}: {
    hasMultipleCameras: boolean;
    facingMode: 'user' | 'environment';
    showFilters: boolean;
    onSwitchCamera: () => void;
    onToggleFilters: () => void;
}) {
    return (
        <div className="camera-controls-top">
            {hasMultipleCameras && (
                <button
                    type="button"
                    className="camera-switch-btn"
                    onClick={onSwitchCamera}
                    aria-label={`Switch to ${facingMode === 'user' ? 'back' : 'front'} camera`}
                >
                    🔄
                </button>
            )}
            <button
                type="button"
                className={`filter-toggle-btn${showFilters ? ' active' : ''}`}
                onClick={onToggleFilters}
                aria-label={showFilters ? 'Hide lenses' : 'Show lenses'}
                aria-expanded={showFilters}
            >
                <span aria-hidden="true">🎭</span>
                <span>{showFilters ? 'Hide' : 'Lenses'}</span>
            </button>
        </div>
    );
}

export function CameraErrorPanel({
    error,
    hasPhoto,
    onRetry,
    onUseFile,
}: {
    error: string;
    hasPhoto: boolean;
    onRetry: () => void;
    onUseFile: () => void;
}) {
    if (!error) return null;
    return (
        <div className="camera-error">
            <p>{error}</p>
            {!hasPhoto && (
                <>
                    <button className="btn btn-primary" onClick={onRetry}>
                        Try Again
                    </button>
                    <button className="btn btn-outline file-fallback-btn" onClick={onUseFile}>
                        Upload from device
                    </button>
                </>
            )}
        </div>
    );
}

export function PreviewActions({
    uploading,
    onRetake,
    onSend,
}: {
    uploading: boolean;
    onRetake: () => void;
    onSend: () => void;
}) {
    return (
        <div className="preview-controls">
            <button className="btn btn-outline" onClick={onRetake} disabled={uploading}>
                Retake
            </button>
            <button className="btn btn-primary" onClick={onSend} disabled={uploading}>
                {uploading ? 'Sending...' : 'Send 📸'}
            </button>
        </div>
    );
}
