import { useCallback, useEffect, useRef, useState } from 'react';
import type { FaceFrame } from '../lenses/facePose';
import type { FaceTracker as FaceTrackerInstance } from '../lenses/faceTracker';
import type { LensRenderer as LensRendererInstance } from '../lenses/LensRenderer';
import type { LensId } from '../lenses/lensCatalog';

/** Owns the lens-effects lifecycle: renderer/tracker creation, the tracking
 *  animation frame, attempt generations (so a stale async effect can never
 *  attach a renderer/tracker after a restart or unmount), readiness, and
 *  disposal. The selected lens is stored in a ref so capture and upload can
 *  read the current value without re-rendering. */
export function useLensEffects() {
    const overlayCanvasRef = useRef<HTMLCanvasElement>(null);
    const trackerRef = useRef<FaceTrackerInstance | null>(null);
    const rendererRef = useRef<LensRendererInstance | null>(null);
    const trackingAnimationRef = useRef<number | null>(null);
    const lastFrameRef = useRef<FaceFrame | null>(null);
    const selectedFilterRef = useRef<LensId>('none');
    const faceDetectedRef = useRef(false);
    const effectAttemptRef = useRef(0);
    const [selectedFilter, setSelectedFilter] = useState<LensId>('none');
    const [filterReady, setFilterReady] = useState(false);
    const [filterError, setFilterError] = useState('');
    const [faceDetected, setFaceDetected] = useState(false);

    const updateFaceDetected = useCallback((detected: boolean) => {
        if (faceDetectedRef.current === detected) return;
        faceDetectedRef.current = detected;
        setFaceDetected(detected);
    }, []);

    const clearEffects = useCallback(() => {
        lastFrameRef.current = null;
        rendererRef.current?.clear();
        updateFaceDetected(false);
    }, [updateFaceDetected]);

    /** Bumps the effect generation, cancels the tracking animation frame, and
     *  disposes the renderer/tracker. Idempotent, so it is safe to call on
     *  every restart, capture, retake, and unmount. */
    const destroyEffects = useCallback(() => {
        effectAttemptRef.current += 1;
        if (trackingAnimationRef.current !== null) cancelAnimationFrame(trackingAnimationRef.current);
        trackingAnimationRef.current = null;
        clearEffects();
        trackerRef.current?.close();
        rendererRef.current?.dispose();
        trackerRef.current = null;
        rendererRef.current = null;
        setFilterReady(false);
    }, [clearEffects]);

    const createRenderer = useCallback(
        async (
            source: HTMLVideoElement | HTMLCanvasElement,
            width: number,
            height: number,
        ): Promise<LensRendererInstance | null> => {
            const canvas = overlayCanvasRef.current;
            if (!canvas) return null;
            try {
                const { LensRenderer } = await import('../lenses/LensRenderer');
                const renderer = new LensRenderer(canvas);
                renderer.setSource(source);
                renderer.resize(width, height);
                renderer.setLens(selectedFilterRef.current);
                // A newer attempt can create a renderer while an earlier one is
                // still attached (e.g. a lens selected mid-initialization).
                // Dispose the replaced renderer so no WebGL context leaks.
                const previous = rendererRef.current;
                if (previous && previous !== renderer) previous.dispose();
                rendererRef.current = renderer;
                return renderer;
            } catch {
                setFilterError('Camera effects require WebGL. Photos can still be sent without a lens.');
                return null;
            }
        },
        [],
    );

    const initializeVideoEffects = useCallback(
        async (video: HTMLVideoElement, width: number, height: number) => {
            const attempt = ++effectAttemptRef.current;
            setFilterReady(false);
            setFilterError('');
            const renderer = await createRenderer(video, width, height);
            if (!renderer) return;
            if (attempt !== effectAttemptRef.current) {
                renderer.dispose();
                if (rendererRef.current === renderer) rendererRef.current = null;
                return;
            }
            try {
                const { FaceTracker } = await import('../lenses/faceTracker');
                const tracker = await FaceTracker.create();
                if (attempt !== effectAttemptRef.current) {
                    tracker.close();
                    return;
                }
                trackerRef.current = tracker;
                setFilterReady(true);
                let lastVideoTime = -1;
                const track = (timestamp: number) => {
                    if (attempt !== effectAttemptRef.current) return;
                    if (video.readyState >= 2 && video.currentTime !== lastVideoTime) {
                        lastVideoTime = video.currentTime;
                        try {
                            const frame = tracker.detectVideo(video, timestamp);
                            lastFrameRef.current = frame;
                            updateFaceDetected(Boolean(frame));
                            renderer.render(frame, timestamp);
                        } catch {
                            setFilterError('Face tracking stopped unexpectedly. Try reopening the camera.');
                            updateFaceDetected(false);
                            return;
                        }
                    } else {
                        renderer.render(lastFrameRef.current, timestamp);
                    }
                    trackingAnimationRef.current = requestAnimationFrame(track);
                };
                trackingAnimationRef.current = requestAnimationFrame(track);
            } catch {
                if (attempt === effectAttemptRef.current) {
                    renderer.dispose();
                    rendererRef.current = null;
                    setFilterError('Face tracking could not start. Photos can still be sent without a lens.');
                }
            }
        },
        [createRenderer, updateFaceDetected],
    );

    const initializeImageEffects = useCallback(
        async (source: HTMLCanvasElement, width: number, height: number) => {
            const attempt = ++effectAttemptRef.current;
            setFilterReady(false);
            setFilterError('');
            const renderer = await createRenderer(source, width, height);
            if (!renderer) return;
            if (attempt !== effectAttemptRef.current) {
                renderer.dispose();
                if (rendererRef.current === renderer) rendererRef.current = null;
                return;
            }
            try {
                const { FaceTracker } = await import('../lenses/faceTracker');
                const tracker = await FaceTracker.create();
                if (attempt !== effectAttemptRef.current) {
                    tracker.close();
                    return;
                }
                trackerRef.current = tracker;
                const frame = await tracker.detectImage(source);
                if (attempt !== effectAttemptRef.current) return;
                lastFrameRef.current = frame;
                updateFaceDetected(Boolean(frame));
                const animate = (timestamp: number) => {
                    if (attempt !== effectAttemptRef.current) return;
                    renderer.render(frame, timestamp);
                    trackingAnimationRef.current = requestAnimationFrame(animate);
                };
                animate(performance.now());
                setFilterReady(true);
                if (!frame) setFilterError('No face found. Try a brighter, front-facing photo.');
            } catch {
                if (attempt === effectAttemptRef.current) {
                    setFilterError('This photo could not be tracked. The original can still be sent.');
                }
            }
        },
        [createRenderer, updateFaceDetected],
    );

    // Disposal on unmount so no tracker, renderer, or animation frame outlives
    // the component. Combined with the session hook's stream cleanup, every
    // media resource has exactly one owner and one cleanup path.
    useEffect(
        () => () => {
            destroyEffects();
        },
        [destroyEffects],
    );

    return {
        overlayCanvasRef,
        trackerRef,
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
    };
}
