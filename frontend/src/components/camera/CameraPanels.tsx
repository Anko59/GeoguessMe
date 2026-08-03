export function CameraTopControls({
    hasMultipleCameras,
    facingMode,
    showFilters,
    showOptions,
    onSwitchCamera,
    onToggleFilters,
    onToggleOptions,
}: {
    hasMultipleCameras: boolean;
    facingMode: 'user' | 'environment';
    showFilters: boolean;
    showOptions: boolean;
    onSwitchCamera: () => void;
    onToggleFilters: () => void;
    onToggleOptions: () => void;
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
            <button
                type="button"
                className={`options-toggle-btn${showOptions ? ' active' : ''}`}
                onClick={onToggleOptions}
                aria-label="Challenge options"
                aria-expanded={showOptions}
            >
                <span aria-hidden="true">⚙️</span>
                <span>{showOptions ? 'Hide' : 'Options'}</span>
            </button>
        </div>
    );
}

export function CameraOptionsMenu({
    groups,
    selectedGroupIDs,
    hideLocation,
    onToggleGroup,
    onToggleHideLocation,
    onClose,
}: {
    groups: { id: string; name: string }[];
    selectedGroupIDs: string[];
    hideLocation: boolean;
    onToggleGroup: (id: string) => void;
    onToggleHideLocation: () => void;
    onClose: () => void;
}) {
    return (
        <div className="camera-options-menu" role="dialog" aria-label="Challenge options">
            <div className="camera-options-section">
                <p className="camera-options-title">Send to</p>
                {groups.length === 0 ? (
                    <p className="camera-options-hint">Loading your groups…</p>
                ) : (
                    <div className="camera-options-groups">
                        {groups.map((group) => (
                            <label key={group.id} className="camera-options-group">
                                <input
                                    type="checkbox"
                                    checked={selectedGroupIDs.includes(group.id)}
                                    onChange={() => onToggleGroup(group.id)}
                                />
                                <span>{group.name}</span>
                            </label>
                        ))}
                    </div>
                )}
            </div>
            <label className="camera-options-hide">
                <input type="checkbox" checked={hideLocation} onChange={onToggleHideLocation} />
                <span>
                    <strong>Hide my location</strong>
                    <small>Guessers see only distances; the spot is revealed after 48 hours.</small>
                </span>
            </label>
            <button type="button" className="btn btn-secondary camera-options-close" onClick={onClose}>
                Done
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
                {uploading ? 'Sending...' : 'Send'}
            </button>
        </div>
    );
}
