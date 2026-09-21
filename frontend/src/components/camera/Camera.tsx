import { useCallback, useEffect, useRef, useState } from 'react';
import { isFilterableImageType } from './cameraUtils';
import { useCameraSession } from './lifecycle/useCameraSession';
import { useLensEffects } from './lifecycle/useLensEffects';
import { useChallengeOptions, useChallengeUpload, type CaptureUploadOptions } from './useChallengeUpload';
import { useMediaProcessingJob, mediaProcessingErrorMessage } from '../../hooks/useMediaProcessingJob';
import type { MediaProcessingJob } from '../../types';
import './Camera.css';
import CameraView from './CameraView';
import { capturePhotoFrame, prepareImageForFilters } from './capture/cameraImagePreparation';
import type { LensId } from './lenses/lensCatalog';
import { EMPTY_TEXT_BANNER, type TextBanner } from './textBanner';
import { useHoldToRecord } from './capture/useHoldToRecord';
import { useVideoCapture } from './capture/useVideoCapture';
import { useFaceTrackerPreload } from './lenses/useFaceTrackerPreload';
import { captureFeedback } from '../../platform/haptics';

const FLASH_DURATION_MS = 300;

function createIdempotencyKey(): string {
    const cryptoAPI = globalThis.crypto;
    if (cryptoAPI?.randomUUID) return cryptoAPI.randomUUID();
    if (cryptoAPI?.getRandomValues) {
        const bytes = cryptoAPI.getRandomValues(new Uint8Array(16));
        bytes[6] = (bytes[6] & 0x0f) | 0x40;
        bytes[8] = (bytes[8] & 0x3f) | 0x80;
        const hex = [...bytes].map((byte) => byte.toString(16).padStart(2, '0')).join('');
        return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
    }
    throw new Error('Secure random values are required to publish a challenge.');
}

export interface CameraProps {
    /** Group challenge destination. Omit when using the feed destination. */
    groupID?: string;
    onUploadComplete: () => void;
    /** Override the group upload with a destination such as the public feed. */
    uploadCaptured?: (
        blob: Blob,
        filename: string,
        position: GeolocationPosition,
        options: CaptureUploadOptions,
    ) => Promise<MediaProcessingJob | null>;
    /** Feed capture is camera-only and does not expose group-only controls. */
    variant?: 'group' | 'feed';
}

export default function Camera({ groupID = '', onUploadComplete, uploadCaptured, variant = 'group' }: CameraProps) {
    const feedMode = variant === 'feed';
    const allowFileFallback = !feedMode;
    const allowVideo = !feedMode;
    const [capturedPhoto, setCapturedPhoto] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');
    const [fileMode, setFileMode] = useState(false);
    const [flashVisible, setFlashVisible] = useState(false);
    const [processingJobID, setProcessingJobID] = useState<string | null>(null);
    const [textBanner, setTextBanner] = useState<TextBanner>(EMPTY_TEXT_BANNER);
    const [idempotencyKey, setIdempotencyKey] = useState(createIdempotencyKey);
    const [showFilters, setShowFilters] = useState(
        () => !window.matchMedia('(pointer: coarse), (max-width: 40rem)').matches,
    );
    const {
        showOptions,
        availableGroups,
        targetGroupIDs,
        hideLocation,
        toggleOptions,
        toggleGroup,
        toggleHideLocation,
        audience,
        setAudience,
        caption,
        setCaption,
        closeOptions,
    } = useChallengeOptions(feedMode ? '' : groupID, feedMode);
    const captureCanvasRef = useRef<HTMLCanvasElement>(null);
    const sourceCanvasRef = useRef<HTMLCanvasElement>(null);
    const fileInputRef = useRef<HTMLInputElement>(null);
    const preparedFileDataRef = useRef<string | null>(null);
    const filePreparationAttemptRef = useRef(0);
    const locationRequestRef = useRef<Promise<GeolocationPosition> | null>(null);
    const flashTimerRef = useRef<number | null>(null);

    const lens = useLensEffects();
    const {
        overlayCanvasRef,
        rendererRef,
        lastFrameRef,
        selectedFilterRef,
        selectedFilter,
        setSelectedFilter,
        filterReady,
        filterError,
        setFilterError,
        faceDetected,
        destroyEffects,
        initializeVideoEffects,
        initializeImageEffects,
    } = lens;

    const recordingError = useCallback((message: string) => setError(message), []);
    const capturedVideoError = useCallback(
        () => setError('The recorded video could not be played. Please record a new clip and try again.'),
        [],
    );
    const { recordedVideo, recording, startHeldRecording, stopRecording, discardRecording } = useVideoCapture({
        onError: recordingError,
    });
    useFaceTrackerPreload();

    const session = useCameraSession({
        onReset: useCallback(() => {
            filePreparationAttemptRef.current += 1;
            preparedFileDataRef.current = null;
            setFileMode(false);
            setCapturedPhoto(null);
            discardRecording();
            setFilterError('');
            if (sourceCanvasRef.current) sourceCanvasRef.current.width = 0;
            destroyEffects();
        }, [discardRecording, destroyEffects, setFilterError]),
        onReady: useCallback(
            (video: HTMLVideoElement, width: number, height: number) => {
                if (selectedFilterRef.current !== 'none') void initializeVideoEffects(video, width, height);
            },
            [initializeVideoEffects, selectedFilterRef],
        ),
        setError,
        allowFileFallback,
    });
    const { videoRef, streamRef, cameraReady, startCamera, stopCamera, facingMode, hasMultipleCameras, switchCamera } =
        session;

    // Declarative capture flash: one timer owner with cleanup on unmount.
    useEffect(
        () => () => {
            if (flashTimerRef.current !== null) window.clearTimeout(flashTimerRef.current);
        },
        [],
    );

    const startHeldVideo = async (isStillPressed: () => boolean) => {
        if (recording) return;
        const videoStream = streamRef.current;
        if (!videoStream) return;
        setError('');
        await startHeldRecording(
            videoStream,
            isStillPressed,
            () => {
                destroyEffects();
                stopCamera();
            },
            facingMode === 'user',
            videoRef.current,
        );
    };

    const capturePhoto = () => {
        const photo = capturePhotoFrame({
            video: videoRef.current,
            overlay: overlayCanvasRef.current,
            captureCanvas: captureCanvasRef.current,
            sourceCanvas: sourceCanvasRef.current,
            renderer: rendererRef.current,
            frame: lastFrameRef.current,
            // Front cameras preview mirrored, so the captured photo must be
            // flipped to match what the user saw; back cameras stay as-is.
            mirror: facingMode === 'user',
        });
        if (!photo) return;
        if (flashTimerRef.current !== null) window.clearTimeout(flashTimerRef.current);
        setFlashVisible(true);
        flashTimerRef.current = window.setTimeout(() => {
            flashTimerRef.current = null;
            setFlashVisible(false);
        }, FLASH_DURATION_MS);
        setCapturedPhoto(photo);
        void captureFeedback();
        destroyEffects();
        stopCamera();
    };

    const retake = () => {
        // A retake is a new captured challenge. Keep the current key only for
        // retries of this exact captured send so a later capture cannot replay
        // the earlier publication.
        setIdempotencyKey(createIdempotencyKey());
        setCapturedPhoto(null);
        discardRecording();
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

    const prepareImageFilter = (dataURL: string) => {
        const preparationAttempt = filePreparationAttemptRef.current;
        prepareImageForFilters({
            dataURL,
            isCurrent: () => preparationAttempt === filePreparationAttemptRef.current,
            sourceCanvas: sourceCanvasRef.current,
            onPrepared: async (sourceCanvas, width, height) => {
                preparedFileDataRef.current = dataURL;
                if (selectedFilterRef.current !== 'none') await initializeImageEffects(sourceCanvas, width, height);
            },
            onError: () => {
                preparedFileDataRef.current = null;
                setError('Failed to read the selected file.');
            },
        });
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
        reader.onerror = () => {
            preparedFileDataRef.current = null;
            setError('Failed to read the selected file.');
        };
        reader.readAsDataURL(file);
    };

    const selectLens = (lensId: LensId) => {
        selectedFilterRef.current = lensId;
        setSelectedFilter(lensId);
        if (lensId === 'none') {
            destroyEffects();
            return;
        }
        if (rendererRef.current) {
            rendererRef.current.setLens(lensId);
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

    const captureGesture = useHoldToRecord({
        onHold: allowVideo ? startHeldVideo : async () => undefined,
        onStop: stopRecording,
        onTap: capturePhoto,
        enableHold: allowVideo,
    });
    // While recording, tapping the capture button stops the clip instead of taking a photo.
    const captureButtonClick = recording ? stopRecording : captureGesture.onClick;

    const completeUpload = useCallback(() => {
        setIdempotencyKey(createIdempotencyKey());
        onUploadComplete();
    }, [onUploadComplete]);

    const { requestLocation, handleUpload } = useChallengeUpload({
        groupIDs: targetGroupIDs,
        hideLocation,
        audience,
        caption,
        idempotencyKey,
        uploadCaptured,
        fileMode,
        capturedPhoto,
        textBanner,
        recordedVideo,
        sourceCanvasRef,
        captureCanvasRef,
        overlayCanvasRef,
        rendererRef,
        lastFrameRef,
        preparedFileDataRef,
        locationRequestRef,
        destroyEffects,
        stopCamera,
        discardRecording,
        onUploadComplete: completeUpload,
        setCapturedPhoto,
        setFileMode,
        setError,
        setUploading,
        setProcessingJobID,
    });

    // Poll an asynchronous video-processing job to completion. The status
    // endpoint is owner-only and never exposes storage keys or raw upload
    // metadata; the poll itself stops on completion, unmount, or logout.
    // Terminal transitions are handled through the hook callbacks (fired from
    // the async poll) rather than by an effect that sets state synchronously.
    const handleJobReady = useCallback(() => {
        setProcessingJobID(null);
        completeUpload();
    }, [completeUpload, setProcessingJobID]);

    const handleJobFailed = useCallback(
        (job: MediaProcessingJob) => {
            setProcessingJobID(null);
            setError(mediaProcessingErrorMessage(job.error_code));
        },
        [setError, setProcessingJobID],
    );

    const handleJobUnavailable = useCallback(
        (message: string) => {
            setProcessingJobID(null);
            setError(message);
        },
        [setError, setProcessingJobID],
    );

    useMediaProcessingJob(processingJobID, {
        onReady: handleJobReady,
        onFailed: handleJobFailed,
        onUnavailable: handleJobUnavailable,
    });

    useEffect(() => {
        void requestLocation();
    }, [requestLocation]);

    return (
        <>
            <CameraView
                videoRef={videoRef}
                overlayCanvasRef={overlayCanvasRef}
                captureCanvasRef={captureCanvasRef}
                sourceCanvasRef={sourceCanvasRef}
                fileInputRef={fileInputRef}
                cameraReady={cameraReady}
                capturedPhoto={capturedPhoto}
                capturedVideo={recordedVideo?.url ?? null}
                recording={recording}
                fileMode={fileMode}
                error={error}
                hasMultipleCameras={hasMultipleCameras}
                facingMode={facingMode}
                showFilters={showFilters}
                showOptions={showOptions}
                optionsGroups={availableGroups}
                selectedGroupIDs={targetGroupIDs}
                hideLocation={hideLocation}
                selectedFilter={selectedFilter}
                filterReady={filterReady}
                filterError={filterError}
                faceDetected={faceDetected}
                textBanner={textBanner}
                uploading={uploading}
                processingVideo={processingJobID !== null}
                allowFileFallback={allowFileFallback}
                allowVideo={allowVideo}
                showChallengeOptions
                feedMode={feedMode}
                feedAudience={audience}
                feedCaption={caption}
                captureSummary={
                    feedMode
                        ? `${audience === 'public' ? 'Public' : 'Friends'} · ${targetGroupIDs.length} group${targetGroupIDs.length === 1 ? '' : 's'}`
                        : undefined
                }
                onStartCamera={() => void startCamera()}
                onSetFileMode={() => setFileMode(true)}
                onSwitchCamera={switchCamera}
                onToggleFilters={() => setShowFilters((p) => !p)}
                onToggleOptions={toggleOptions}
                onToggleGroup={toggleGroup}
                onToggleHideLocation={toggleHideLocation}
                onAudienceChange={setAudience}
                onCaptionChange={setCaption}
                onCloseOptions={closeOptions}
                onSelectLens={selectLens}
                onBannerChange={setTextBanner}
                onCaptureButtonClick={captureButtonClick}
                onCaptureButtonPointerDown={captureGesture.onPointerDown}
                onCaptureButtonPointerUp={captureGesture.onPointerUp}
                onCaptureButtonPointerCancel={captureGesture.onPointerCancel}
                onFileSelected={handleFileSelected}
                onUpload={() => void handleUpload()}
                onRetake={retake}
                onCapturedVideoError={capturedVideoError}
            />
            {flashVisible && <div className="camera-flash" aria-hidden="true" />}
        </>
    );
}
