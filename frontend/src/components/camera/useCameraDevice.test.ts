import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useCameraDevice } from './useCameraDevice';

const mocks = vi.hoisted(() => ({
    enumerateDevices: vi.fn(),
}));

beforeEach(() => {
    vi.clearAllMocks();
    mocks.enumerateDevices.mockReset();
    vi.stubGlobal('navigator', {
        mediaDevices: { enumerateDevices: mocks.enumerateDevices },
    });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

describe('useCameraDevice', () => {
    it('defaults to user-facing camera with a single device', async () => {
        mocks.enumerateDevices.mockResolvedValue([
            { deviceId: 'cam1', kind: 'videoinput', label: 'Front Camera', groupId: 'g1' },
        ]);
        const { result } = renderHook(() => useCameraDevice());
        expect(result.current.facingMode).toBe('user');
        await act(async () => {
            await Promise.resolve();
        });
        expect(result.current.hasMultipleCameras).toBe(false);
    });

    it('detects multiple cameras', async () => {
        mocks.enumerateDevices.mockResolvedValue([
            { deviceId: 'cam1', kind: 'videoinput', label: 'Front Camera', groupId: 'g1' },
            { deviceId: 'cam2', kind: 'videoinput', label: 'Back Camera', groupId: 'g2' },
        ]);
        const { result } = renderHook(() => useCameraDevice());
        await act(async () => {
            await Promise.resolve();
        });
        expect(result.current.hasMultipleCameras).toBe(true);
    });

    it('switchCamera toggles facing mode and calls the stored restart', async () => {
        mocks.enumerateDevices.mockResolvedValue([
            { deviceId: 'cam1', kind: 'videoinput', label: 'Front Camera', groupId: 'g1' },
            { deviceId: 'cam2', kind: 'videoinput', label: 'Back Camera', groupId: 'g2' },
        ]);
        const { result } = renderHook(() => useCameraDevice());
        await act(async () => {
            await Promise.resolve();
        });
        const restart = vi.fn().mockResolvedValue(undefined);
        act(() => {
            result.current.setRestart(restart);
        });

        act(() => {
            result.current.switchCamera();
        });
        expect(result.current.facingMode).toBe('environment');
        expect(result.current.facingModeRef.current).toBe('environment');
        expect(restart).toHaveBeenCalledOnce();

        act(() => {
            result.current.switchCamera();
        });
        expect(result.current.facingMode).toBe('user');
        expect(restart).toHaveBeenCalledTimes(2);
    });

    it('handles enumerateDevices rejection gracefully', () => {
        mocks.enumerateDevices.mockRejectedValue(new Error('NotAllowed'));
        const { result } = renderHook(() => useCameraDevice());
        expect(result.current.hasMultipleCameras).toBe(false);
        expect(result.current.facingMode).toBe('user');
    });

    it('handles absent enumerateDevices API', () => {
        vi.stubGlobal('navigator', { mediaDevices: {} });
        const { result } = renderHook(() => useCameraDevice());
        expect(result.current.hasMultipleCameras).toBe(false);
    });
});
