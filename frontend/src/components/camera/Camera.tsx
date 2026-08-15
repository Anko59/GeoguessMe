import { useCallback, useEffect, useRef, useState } from 'react';
import { isFilterableImageType } from './cameraUtils';
import { useCameraSession } from './lifecycle/useCameraSession';
import { useLensEffects } from './lifecycle/useLensEffects';
import { useChallengeOptions, useChallengeUpload } from './useChallengeUpload';
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

const FLASH_DURATION_MS = 300;

export default function Camera({ groupID, onUploadComplete }: { groupID: string; onUploadComplete: () => void }) {
    const [capturedPhoto, setCapturedPhoto] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [error, setError] = useState('');
    const [fileMode, setFileMode] = useState(false);
    const [flashVisible, setFlashVisible] = useState(false);
    const [processingJobID, setProcessingJobID] = useState<string | null>(null);
    const [textBanner, setTextBanner] = useState<TextBanner>(EMPTY_TEXT_BANNER);
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
        closeOptions,
    } = useChallengeOptions(groupID);
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
        destroyEffects();
        stopCamera();
    };

    const retake = () => {
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
        onHold: startHeldVideo,
        onStop: stopRecording,
        onTap: capturePhoto,
    });
    // While recording, tapping the capture button stops the clip instead of taking a photo.
    const captureButtonClick = recording ? stopRecording : captureGesture.onClick;

    const { requestLocation, handleUpload } = useChallengeUpload({
        groupIDs: targetGroupIDs,
        hideLocation,
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
        onUploadComplete,
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
        onUploadComplete();
    }, [onUploadComplete, setProcessingJobID]);

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
                onStartCamera={() => void startCamera()}
                onSetFileMode={() => setFileMode(true)}
                onSwitchCamera={switchCamera}
                onToggleFilters={() => setShowFilters((p) => !p)}
                onToggleOptions={toggleOptions}
                onToggleGroup={toggleGroup}
                onToggleHideLocation={toggleHideLocation}
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
