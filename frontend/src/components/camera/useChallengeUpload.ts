import { useCallback, useEffect, useRef, useState } from 'react';
import api from '../../api';
import type { Group } from '../../types';
import type React from 'react';
import { getAPIErrorMessage } from '../../api';
import { dataURLToBlob, getCurrentPosition, uploadPhoto } from './cameraUtils';
import { drawTextBanner, type TextBanner } from './textBanner';
import type { FaceFrame } from './lenses/facePose';
import type { LensRenderer as LensRendererInstance } from './lenses/LensRenderer';

interface ChallengeUploadDeps {
    groupIDs: string[];
    hideLocation: boolean;
    fileMode: boolean;
    capturedPhoto: string | null;
    textBanner: TextBanner;
    recordedVideo: { blob: Blob } | null;
    sourceCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    captureCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    overlayCanvasRef: React.RefObject<HTMLCanvasElement | null>;
    rendererRef: React.RefObject<LensRendererInstance | null>;
    lastFrameRef: React.RefObject<FaceFrame | null>;
    preparedFileDataRef: React.MutableRefObject<string | null>;
    locationRequestRef: React.MutableRefObject<Promise<GeolocationPosition> | null>;
    destroyEffects: () => void;
    stopCamera: () => void;
    discardRecording: () => void;
    onUploadComplete: () => void;
    setCapturedPhoto: React.Dispatch<React.SetStateAction<string | null>>;
    setFileMode: React.Dispatch<React.SetStateAction<boolean>>;
    setError: React.Dispatch<React.SetStateAction<string>>;
    setUploading: React.Dispatch<React.SetStateAction<boolean>>;
}

/** Bundles the challenge send flow: a single-flight geolocation request, the
 *  final render of the captured photo, and the upload itself. */
export function useChallengeUpload(deps: ChallengeUploadDeps) {
    const {
        groupIDs,
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
    } = deps;

    const requestLocation = useCallback((): Promise<GeolocationPosition> => {
        if (locationRequestRef.current) return locationRequestRef.current;
        const request = getCurrentPosition();
        locationRequestRef.current = request;
        void request.catch(() => {
            if (locationRequestRef.current === request) locationRequestRef.current = null;
        });
        return request;
    }, [locationRequestRef]);

    const renderFinalPhoto = useCallback((): string | null => {
        const sourceCanvas = sourceCanvasRef.current;
        const captureCanvas = captureCanvasRef.current;
        if (!sourceCanvas || !captureCanvas || sourceCanvas.width === 0) return capturedPhoto;
        const context = captureCanvas.getContext('2d');
        if (!context) return capturedPhoto;
        captureCanvas.width = sourceCanvas.width;
        captureCanvas.height = sourceCanvas.height;
        context.drawImage(sourceCanvas, 0, 0);
        if (fileMode && preparedFileDataRef.current === capturedPhoto && overlayCanvasRef.current) {
            rendererRef.current?.render(lastFrameRef.current);
            context.drawImage(overlayCanvasRef.current, 0, 0, sourceCanvas.width, sourceCanvas.height);
        }
        drawTextBanner(context, captureCanvas.width, captureCanvas.height, textBanner);
        return captureCanvas.toDataURL('image/jpeg', 0.9);
    }, [
        capturedPhoto,
        captureCanvasRef,
        fileMode,
        lastFrameRef,
        overlayCanvasRef,
        preparedFileDataRef,
        rendererRef,
        sourceCanvasRef,
        textBanner,
    ]);

    const handleUpload = useCallback(async (): Promise<void> => {
        const video = recordedVideo;
        const photo = video ? null : renderFinalPhoto();
        if (!video && !photo) return;
        if (groupIDs.length === 0) {
            setError('Select at least one group to send the challenge to.');
            return;
        }
        setUploading(true);
        setError('');
        try {
            const position = await requestLocation();
            if (video) {
                const extension = video.blob.type === 'video/mp4' ? 'mp4' : 'webm';
                await uploadPhoto(video.blob, `capture.${extension}`, groupIDs, position, hideLocation);
            } else {
                await uploadPhoto(
                    dataURLToBlob(photo as string),
                    fileMode ? 'upload.jpg' : 'capture.jpg',
                    groupIDs,
                    position,
                    hideLocation,
                );
            }
            destroyEffects();
            stopCamera();
            setCapturedPhoto(null);
            discardRecording();
            setFileMode(false);
            onUploadComplete();
        } catch (requestError: unknown) {
            const message = requestError instanceof Error ? requestError.message : String(requestError);
            setError(
                /location|geolocation|denied/i.test(message)
                    ? 'Unable to retrieve location. Please enable location services.'
                    : getAPIErrorMessage(requestError, 'Upload failed. Please try again.'),
            );
        } finally {
            setUploading(false);
        }
    }, [
        destroyEffects,
        discardRecording,
        fileMode,
        groupIDs,
        hideLocation,
        onUploadComplete,
        recordedVideo,
        renderFinalPhoto,
        requestLocation,
        setCapturedPhoto,
        setError,
        setFileMode,
        setUploading,
        stopCamera,
    ]);

    return { requestLocation, renderFinalPhoto, handleUpload };
}

// Capture-screen options: which groups receive the challenge and whether the
// exact location stays hidden from guessers.
export function useChallengeOptions(currentGroupID: string) {
    const [showOptions, setShowOptions] = useState(false);
    const [availableGroups, setAvailableGroups] = useState<Group[]>([]);
    const [targetGroupIDs, setTargetGroupIDs] = useState<string[]>([currentGroupID]);
    const [hideLocation, setHideLocation] = useState(false);
    const previousGroupID = useRef(currentGroupID);

    useEffect(() => {
        if (previousGroupID.current === currentGroupID) return;
        previousGroupID.current = currentGroupID;
        setTargetGroupIDs([currentGroupID]);
        setAvailableGroups([]);
        setShowOptions(false);
        setHideLocation(false);
    }, [currentGroupID]);

    const toggleOptions = useCallback(() => {
        setShowOptions((open) => {
            if (!open) {
                void api
                    .get<Group[]>('/user/groups')
                    .then((response) => setAvailableGroups(response.data))
                    .catch(() => setAvailableGroups([]));
            }
            return !open;
        });
    }, []);

    const toggleGroup = useCallback((id: string) => {
        setTargetGroupIDs((current) =>
            current.includes(id) ? current.filter((groupID) => groupID !== id) : [...current, id],
        );
    }, []);

    const toggleHideLocation = useCallback(() => setHideLocation((current) => !current), []);
    const closeOptions = useCallback(() => setShowOptions(false), []);

    return {
        showOptions,
        availableGroups,
        targetGroupIDs,
        hideLocation,
        toggleOptions,
        toggleGroup,
        toggleHideLocation,
        closeOptions,
    };
}
