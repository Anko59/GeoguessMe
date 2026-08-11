import { useEffect, useRef, useState } from 'react';
import api, { getAccessToken } from '../api';
import type { MediaProcessingJob } from '../types';

export interface UseMediaProcessingJobResult {
    job: MediaProcessingJob | null;
    /** Non-sensitive polling error (status unavailable after bounded retries). */
    error: string;
    /** True while a job is queued or processing and polling is active. */
    isProcessing: boolean;
    /** True once a terminal status (ready or failed) was observed. */
    isDone: boolean;
}

/** Terminal callbacks fired from the async poll, never from an effect body. */
export interface UseMediaProcessingJobCallbacks {
    /** Invoked once when the job reaches ready. */
    onReady?: (job: MediaProcessingJob) => void;
    /** Invoked once when the job fails (job carries the stable error code). */
    onFailed?: (job: MediaProcessingJob) => void;
    /** Invoked when polling gives up after bounded transient failures. */
    onUnavailable?: (message: string) => void;
}

/** Bounded polling backoff: 1s, 2s, 3s, 4s, then 5s (capped). */
const BACKOFF_MS = [1_000, 2_000, 3_000, 4_000, 5_000] as const;
const MAX_TRANSIENT_ERRORS = 5;

const TERMINAL_STATUSES: ReadonlySet<MediaProcessingJob['status']> = new Set(['ready', 'failed']);

const UNAVAILABLE_MESSAGE = 'Processing status is temporarily unavailable. Please check back shortly.';

/** Friendly copy for each stable backend failure code; never leaks internals. */
const ERROR_CODE_MESSAGES: Record<string, string> = {
    invalid_video: 'That video could not be read. Please record a new clip and try again.',
    unsupported_codec: 'That video format is not supported. Please record a new clip.',
    too_long: 'Videos must be 30 seconds or shorter. Please record a shorter clip.',
    too_large_dims: 'That video resolution is too large. Please record a smaller clip.',
    too_high_fps: 'That video frame rate is too high. Please record a different clip.',
    too_large: 'That video is too large to process. Please record a shorter clip.',
    output_too_large: 'The processed video is too large. Please record a shorter clip.',
    authorization_revoked: 'You no longer have access to every selected group.',
    transcode_failed: 'The video could not be processed. Please try again.',
    timeout: 'Processing that video took too long. Please try again.',
};

export function mediaProcessingErrorMessage(code: string | undefined): string {
    return (code && ERROR_CODE_MESSAGES[code]) || 'The video could not be processed. Please try again.';
}

/**
 * Polls the owner-only media-processing status endpoint for an asynchronous
 * video job.
 *
 * Polling is bounded: the delay follows 1s -> 2s -> 3s -> 4s -> 5s (capped at
 * 5s) and stops on a terminal status, on unmount, on navigation (unmount),
 * when the job id changes, or when the access token clears on logout. An
 * AbortController cancels any in-flight request when polling stops, and
 * transient network failures retry a bounded number of times before a generic
 * "unavailable" error is surfaced.
 *
 * Polled state is keyed by the job it belongs to, so a new upload never shows
 * the previous job's stale data while its first poll is in flight. All state
 * changes and terminal callbacks happen inside the asynchronous poll callbacks
 * rather than synchronously in the effect body.
 */
export function useMediaProcessingJob(
    jobID: string | null,
    callbacks: UseMediaProcessingJobCallbacks = {},
): UseMediaProcessingJobResult {
    const { onReady, onFailed, onUnavailable } = callbacks;
    // Keep the latest callbacks in refs (updated from an effect) so the poll
    // always invokes a fresh handler without having to restart itself.
    const onReadyRef = useRef(onReady);
    const onFailedRef = useRef(onFailed);
    const onUnavailableRef = useRef(onUnavailable);
    useEffect(() => {
        onReadyRef.current = onReady;
        onFailedRef.current = onFailed;
        onUnavailableRef.current = onUnavailable;
    }, [onReady, onFailed, onUnavailable]);

    const [state, setState] = useState<{ job: MediaProcessingJob | null; error: string }>({
        job: null,
        error: '',
    });
    const [stateJobID, setStateJobID] = useState<string | null>(null);
    const stopRef = useRef(false);
    const controllerRef = useRef<AbortController | null>(null);

    useEffect(() => {
        if (!jobID) return;
        const controller = new AbortController();
        controllerRef.current = controller;
        stopRef.current = false;
        let timer: number | undefined;
        let attempt = 0;
        let transientErrors = 0;

        const scheduleNext = (): void => {
            if (stopRef.current) return;
            const delay = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
            attempt += 1;
            timer = window.setTimeout(() => {
                timer = undefined;
                poll();
            }, delay);
        };

        const poll = (): void => {
            if (stopRef.current) return;
            // Logout signal: the in-memory access token clears on sign-out.
            if (typeof getAccessToken === 'function' && getAccessToken() === null) {
                stopRef.current = true;
                controller.abort();
                return;
            }
            void api
                .get<MediaProcessingJob>(`/media-processing/${encodeURIComponent(jobID)}`, {
                    signal: controller.signal,
                })
                .then((response) => {
                    if (stopRef.current) return;
                    transientErrors = 0;
                    setState({ job: response.data, error: '' });
                    setStateJobID(jobID);
                    if (response.data.status === 'ready') {
                        onReadyRef.current?.(response.data);
                        return;
                    }
                    if (response.data.status === 'failed') {
                        onFailedRef.current?.(response.data);
                        return;
                    }
                    scheduleNext();
                })
                .catch(() => {
                    if (stopRef.current) return;
                    transientErrors += 1;
                    if (transientErrors >= MAX_TRANSIENT_ERRORS) {
                        setState((prev) => ({ ...prev, error: UNAVAILABLE_MESSAGE }));
                        setStateJobID(jobID);
                        onUnavailableRef.current?.(UNAVAILABLE_MESSAGE);
                        return;
                    }
                    scheduleNext();
                });
        };

        poll();

        return () => {
            stopRef.current = true;
            controller.abort();
            if (timer !== undefined) window.clearTimeout(timer);
            controllerRef.current = null;
        };
    }, [jobID]);

    const current = stateJobID === jobID ? state.job : null;
    const currentError = stateJobID === jobID ? state.error : '';
    const isDone = current !== null && TERMINAL_STATUSES.has(current.status);
    return {
        job: current,
        error: currentError,
        isProcessing: jobID !== null && !isDone && currentError === '',
        isDone,
    };
}
