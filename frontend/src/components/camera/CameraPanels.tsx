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
