import { useGroupPhotoUrl } from '../../pages/groups/groupPhotoCache';

/** Deterministic per-group accent so every row in the options menu gets its
 *  own color even before a group photo is uploaded. */
function groupAccentStyle(groupID: string): { background: string } {
    let hash = 0;
    for (let i = 0; i < groupID.length; i += 1) {
        hash = (hash * 31 + groupID.charCodeAt(i)) | 0;
    }
    const hue = Math.abs(hash) % 360;
    return {
        background: `linear-gradient(135deg, hsl(${hue} 62% 42%), hsl(${(hue + 42) % 360} 74% 58%))`,
    };
}

function GroupOption({
    group,
    checked,
    onToggle,
}: {
    group: { id: string; name: string };
    checked: boolean;
    onToggle: () => void;
}) {
    const photoURL = useGroupPhotoUrl(group.id);
    return (
        <li>
            <label className={`camera-options-group${checked ? ' checked' : ''}`}>
                <input type="checkbox" checked={checked} onChange={onToggle} />
                <span className="camera-options-group-icon" style={groupAccentStyle(group.id)} aria-hidden="true">
                    <img src={photoURL} alt="" />
                </span>
                <span className="camera-options-group-name">{group.name}</span>
            </label>
        </li>
    );
}

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
                    <img src="/ui/switch-camera.png" alt="" className="camera-switch-icon" />
                </button>
            )}
            <button
                type="button"
                className={`filter-toggle-btn${showFilters ? ' active' : ''}`}
                onClick={onToggleFilters}
                aria-label={showFilters ? 'Hide lenses' : 'Show lenses'}
                aria-expanded={showFilters}
            >
                <span aria-hidden="true">
                    <img src="/ui/lenses-toggle.png" alt="" className="filter-toggle-icon" />
                </span>
                <span>{showFilters ? 'Hide' : 'Lenses'}</span>
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
                    <ul className="camera-options-groups">
                        {groups.map((group) => (
                            <GroupOption
                                key={group.id}
                                group={group}
                                checked={selectedGroupIDs.includes(group.id)}
                                onToggle={() => onToggleGroup(group.id)}
                            />
                        ))}
                    </ul>
                )}
            </div>
            <label className="camera-options-hide">
                <input type="checkbox" checked={hideLocation} onChange={onToggleHideLocation} />
                <span className="camera-options-hide-icon" aria-hidden="true">
                    <img src="/ui/hide-location.png" alt="" className="camera-options-hide-icon-img" />
                </span>
                <span className="camera-options-hide-text">
                    <strong>Hide my location</strong>
                    <small>Guessers see only distances; the spot is revealed after 48 hours.</small>
                </span>
            </label>
            <button type="button" className="camera-options-done" onClick={onClose}>
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
