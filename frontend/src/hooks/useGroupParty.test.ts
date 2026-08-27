import { act, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PartyStatus } from '../types';
import { useGroupParty } from './useGroupParty';

const mocks = vi.hoisted(() => ({
    get: vi.fn(),
}));

vi.mock('../api', () => ({
    default: { get: mocks.get },
    getAPIErrorMessage: (error: unknown, fallback: string) => (error instanceof Error ? error.message : fallback),
}));

function partyStatus(overrides: Partial<PartyStatus> = {}): PartyStatus {
    return {
        active: false,
        server_time: '2026-06-20T20:00:00Z',
        ...overrides,
    };
}

beforeEach(() => {
    mocks.get.mockReset();
});

afterEach(() => {
    vi.useRealTimers();
});

describe('useGroupParty', () => {
    it('fetches the party state for the group on mount', async () => {
        mocks.get.mockResolvedValue({ data: partyStatus({ active: true, ends_at: '2026-06-20T21:00:00Z' }) });
        const { result } = renderHook(() => useGroupParty('g1'));
        await waitFor(() => expect(result.current.status?.active).toBe(true));
        expect(mocks.get).toHaveBeenCalledWith('/group/party', {
            params: { group_id: 'g1' },
            signal: expect.any(AbortSignal),
        });
        expect(result.current.loading).toBe(false);
    });

    it('holds no status without a group and does not fetch', () => {
        const { result } = renderHook(() => useGroupParty(undefined));
        expect(result.current.status).toBeNull();
        expect(result.current.loading).toBe(false);
        expect(mocks.get).not.toHaveBeenCalled();
    });

    it('drops a stale response when the group changes mid-flight', async () => {
        let resolveSlow: (value: { data: PartyStatus }) => void = () => undefined;
        mocks.get.mockImplementationOnce(
            (_url: string, config?: { signal?: AbortSignal }) =>
                new Promise<{ data: PartyStatus }>((resolve) => {
                    resolveSlow = resolve;
                    config?.signal?.addEventListener('abort', () => undefined);
                }),
        );
        const { result, rerender } = renderHook(
            ({ groupId }: { groupId: string | undefined }) => useGroupParty(groupId),
            {
                initialProps: { groupId: 'g1' as string | undefined },
            },
        );
        await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(1));

        mocks.get.mockResolvedValue({ data: partyStatus({ active: true, ends_at: '2026-06-20T21:00:00Z' }) });
        rerender({ groupId: 'g2' });
        await waitFor(() => expect(result.current.status?.active).toBe(true));
        expect(result.current.status?.active).toBe(true);

        // The slow first response lands after the switch and must be ignored.
        await act(async () => {
            resolveSlow({ data: partyStatus({ active: true, ends_at: '2026-06-20T23:00:00Z' }) });
        });
        expect(Date.parse(result.current.status!.ends_at!)).toBe(Date.parse('2026-06-20T21:00:00Z'));
    });

    it('refresh exposes an event-driven refetch', async () => {
        mocks.get.mockResolvedValue({ data: partyStatus() });
        const { result } = renderHook(() => useGroupParty('g1'));
        await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(1));
        act(() => {
            result.current.refresh();
        });
        await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(2));
    });

    it('flips an active party to ended at its published end and re-syncs', async () => {
        vi.useFakeTimers();
        vi.setSystemTime(Date.parse('2026-06-20T20:00:00Z'));
        mocks.get.mockResolvedValue({
            data: partyStatus({ active: true, ends_at: '2026-06-20T21:00:00Z' }),
        });
        const { result } = renderHook(() => useGroupParty('g1'));
        await act(async () => {
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(result.current.status?.active).toBe(true);

        // One millisecond past the end (corrected for the zero offset) the
        // overlay state flips locally and a fresh fetch confirms.
        mocks.get.mockResolvedValue({ data: partyStatus() });
        await act(async () => {
            await vi.advanceTimersByTimeAsync(60 * 60 * 1000 + 5);
            // Flush the confirmation fetch triggered by the expiry.
            await vi.advanceTimersByTimeAsync(0);
        });
        expect(result.current.status?.active).toBe(false);
        expect(result.current.status?.ends_at).toBeUndefined();
        expect(mocks.get).toHaveBeenCalledTimes(2);
    });

    it('keeps the previous status when a refresh fails', async () => {
        // Future-dated window: no expiry timer fires during the assertion
        // window, so the failed refresh is observed in isolation.
        const now = Date.now();
        mocks.get.mockResolvedValueOnce({
            data: partyStatus({
                active: true,
                ends_at: new Date(now + 3_600_000).toISOString(),
                server_time: new Date(now).toISOString(),
            }),
        });
        const { result } = renderHook(() => useGroupParty('g1'));
        await waitFor(() => expect(result.current.status?.active).toBe(true));
        expect(result.current.loading).toBe(false);
        mocks.get.mockRejectedValueOnce(new Error('offline'));
        act(() => {
            result.current.refresh();
        });
        await waitFor(() => expect(mocks.get).toHaveBeenCalledTimes(2));
        // A transient failure must not blank a live celebration; the expiry
        // timer and later refreshes self-correct stale state.
        expect(result.current.status?.active).toBe(true);
        await waitFor(() => expect(result.current.loading).toBe(false));
    });
});
