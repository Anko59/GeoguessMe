import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { MediaProcessingJob } from '../types';
import { mediaProcessingErrorMessage, useMediaProcessingJob } from './useMediaProcessingJob';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
    getAccessToken: vi.fn<() => string | null>(() => 'token'),
}));

vi.mock('../api', () => ({
    default: { get: mocks.get },
    getAccessToken: mocks.getAccessToken,
}));

function job(overrides: Partial<MediaProcessingJob> = {}): MediaProcessingJob {
    return {
        id: 'job-1',
        kind: 'challenge',
        status: 'queued',
        queued_at: '2026-08-10T12:00:00Z',
        ...overrides,
    };
}

const JOB_URL = '/media-processing/job-1';

beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    mocks.getAccessToken.mockReturnValue('token');
});

afterEach(() => {
    vi.useRealTimers();
});

describe('useMediaProcessingJob', () => {
    it('polls on a bounded 1s, 2s, 3s, 4s, 5s backoff schedule', async () => {
        mocks.get.mockResolvedValue({ data: job() });
        renderHook(() => useMediaProcessingJob('job-1'));
        expect(mocks.get).toHaveBeenCalledTimes(1);
        expect(mocks.get).toHaveBeenNthCalledWith(1, JOB_URL, expect.anything());

        await act(async () => {
            await vi.advanceTimersByTimeAsync(1_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(2);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(2_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(3);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(3_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(4);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(4_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(5);
        // The backoff caps at 5s.
        await act(async () => {
            await vi.advanceTimersByTimeAsync(5_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(6);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(5_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(7);
    });

    it('stops on a ready status and exposes the result', async () => {
        mocks.get.mockResolvedValue({ data: job({ status: 'ready' }) });
        const { result } = renderHook(() => useMediaProcessingJob('job-1'));
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(result.current.job?.status).toBe('ready');
        expect(result.current.isDone).toBe(true);
        expect(result.current.isProcessing).toBe(false);
        expect(result.current.error).toBe('');

        const calls = mocks.get.mock.calls.length;
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60_000);
        });
        expect(mocks.get.mock.calls.length).toBe(calls);
    });

    it('stops on a failed status and keeps the stable error code', async () => {
        mocks.get.mockResolvedValue({ data: job({ status: 'failed', error_code: 'too_long' }) });
        const { result } = renderHook(() => useMediaProcessingJob('job-1'));
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(result.current.job?.status).toBe('failed');
        expect(result.current.job?.error_code).toBe('too_long');
        expect(result.current.isDone).toBe(true);

        const calls = mocks.get.mock.calls.length;
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60_000);
        });
        expect(mocks.get.mock.calls.length).toBe(calls);
    });

    it('surfaces a generic unavailable error after bounded transient failures', async () => {
        mocks.get.mockRejectedValue(new Error('network down'));
        const { result } = renderHook(() => useMediaProcessingJob('job-1'));
        // Five failures total: the immediate poll plus four retries.
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1_000);
        });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(2_000);
        });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(3_000);
        });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(4_000);
        });
        expect(result.current.error).toContain('temporarily unavailable');
        expect(result.current.job).toBeNull();

        const calls = mocks.get.mock.calls.length;
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60_000);
        });
        expect(mocks.get.mock.calls.length).toBe(calls);
    });

    it('aborts the in-flight request and stops polling on unmount', async () => {
        let signal: AbortSignal | undefined;
        mocks.get.mockImplementation((_url: string, config: { signal?: AbortSignal }) => {
            signal = config.signal;
            return new Promise(() => {});
        });
        const { unmount } = renderHook(() => useMediaProcessingJob('job-1'));
        expect(signal).toBeDefined();
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(signal?.aborted).toBe(false);

        unmount();
        expect(signal?.aborted).toBe(true);
        const calls = mocks.get.mock.calls.length;
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60_000);
        });
        expect(mocks.get.mock.calls.length).toBe(calls);
    });

    it('stops polling when the access token clears on logout', async () => {
        mocks.getAccessToken.mockReturnValueOnce('token').mockReturnValue(null);
        mocks.get.mockResolvedValue({ data: job() });
        renderHook(() => useMediaProcessingJob('job-1'));

        // The immediate poll runs while the token exists.
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(mocks.get).toHaveBeenCalledTimes(1);

        // The next scheduled poll sees the cleared token and stops without a request.
        await act(async () => {
            await vi.advanceTimersByTimeAsync(1_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(1);
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60_000);
        });
        expect(mocks.get).toHaveBeenCalledTimes(1);
    });

    it('does nothing for a null job id and starts polling once a job id arrives', async () => {
        mocks.get.mockResolvedValue({ data: job() });
        const { rerender } = renderHook(({ id }: { id: string | null }) => useMediaProcessingJob(id), {
            initialProps: { id: null as string | null },
        });
        expect(mocks.get).not.toHaveBeenCalled();
        rerender({ id: 'job-1' });
        expect(mocks.get).toHaveBeenCalledTimes(1);
    });

    it('maps stable failure codes to friendly, non-sensitive copy', () => {
        expect(mediaProcessingErrorMessage('too_long')).toContain('30 seconds or shorter');
        expect(mediaProcessingErrorMessage('transcode_failed')).toContain('could not be processed');
        expect(mediaProcessingErrorMessage(undefined)).toContain('could not be processed');
        expect(mediaProcessingErrorMessage('unexpected_internal_code')).toContain('could not be processed');
    });
});
