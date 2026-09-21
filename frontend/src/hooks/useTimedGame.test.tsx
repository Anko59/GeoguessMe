import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { ChallengeResults } from '../types';
import type { TimedGameAdapter, TimedGameWindow } from './useTimedGame';
import { useTimedGame } from './useTimedGame';

const results: ChallengeResults = {
    photo_id: 'challenge-a',
    group_id: 'feed',
    actual_lat: 48.8,
    actual_long: 2.3,
    guesses: [],
    media_available: false,
    server_time: new Date().toISOString(),
};

function windowFor(offsetMs: number): TimedGameWindow {
    const now = Date.now();
    return {
        viewExpiresAt: new Date(now + offsetMs).toISOString(),
        guessExpiresAt: new Date(now + offsetMs + 120_000).toISOString(),
        scoreGraceSeconds: 30,
        serverTime: new Date(now).toISOString(),
    };
}

function adapterWith(overrides: Partial<TimedGameAdapter> = {}): TimedGameAdapter {
    return {
        loadResults: vi.fn(async () => ({ results })),
        accept: vi.fn(async () => windowFor(120_000)),
        loadMedia: vi.fn(async () => ({ url: 'https://example.test/photo.jpg' })),
        mediaDelivered: vi.fn(async () => windowFor(120_000)),
        guess: vi.fn(async () => ({ score: 4200, duplicate: false })),
        timeout: vi.fn(async () => undefined),
        ...overrides,
    };
}

function renderGame(adapter: TimedGameAdapter, challengeId: string, onStatusChange = vi.fn()) {
    return renderHook(
        ({ id }: { id: string }) =>
            useTimedGame({
                challengeId: id,
                currentUserId: 'user-1',
                isOwner: false,
                requiresCurrentUser: true,
                checkResultsBeforeAccept: false,
                adapter,
                onStatusChange,
                onClose: vi.fn(),
            }),
        { initialProps: { id: challengeId } },
    );
}

describe('useTimedGame operation lifecycle', () => {
    it('aborts a pending guess and ignores its late response after unmount', async () => {
        let resolveGuess!: (value: { score: number; duplicate: boolean }) => void;
        let guessSignal: AbortSignal | undefined;
        const guessPromise = new Promise<{ score: number; duplicate: boolean }>((resolve) => {
            resolveGuess = resolve;
        });
        const onStatusChange = vi.fn();
        const adapter = adapterWith({
            loadMedia: vi.fn(async () => {
                throw new Error('media window elapsed');
            }),
            accept: vi.fn(async () => windowFor(-1000)),
            guess: vi.fn((_id, _point, signal) => {
                guessSignal = signal;
                return guessPromise;
            }),
        });
        const hook = renderGame(adapter, 'challenge-a', onStatusChange);
        await waitFor(() => expect(hook.result.current.state.status).toBe('guessing'));
        act(() => hook.result.current.selectLocation({ lat: 48.8, long: 2.3 }));
        await waitFor(() => expect(hook.result.current.state.selectedLocation).toEqual({ lat: 48.8, long: 2.3 }));
        act(() => hook.result.current.submitGuess());
        await waitFor(() => expect(adapter.guess).toHaveBeenCalled());

        hook.unmount();
        expect(guessSignal?.aborted).toBe(true);
        resolveGuess({ score: 4200, duplicate: false });
        await act(async () => {
            await Promise.resolve();
        });

        expect(onStatusChange).not.toHaveBeenCalledWith('challenge-a', 'guessed');
        expect(adapter.loadResults).not.toHaveBeenCalled();
    });

    it('aborts the old operation and ignores its response after switching challenges', async () => {
        let resolveGuess!: (value: { score: number; duplicate: boolean }) => void;
        let guessSignal: AbortSignal | undefined;
        const guessPromise = new Promise<{ score: number; duplicate: boolean }>((resolve) => {
            resolveGuess = resolve;
        });
        const onStatusChange = vi.fn();
        const adapter = adapterWith({
            loadMedia: vi.fn(async () => {
                throw new Error('media window elapsed');
            }),
            accept: vi.fn(async () => windowFor(-1000)),
            guess: vi.fn((_id, _point, signal) => {
                guessSignal = signal;
                return guessPromise;
            }),
        });
        const hook = renderGame(adapter, 'challenge-a', onStatusChange);
        await waitFor(() => expect(hook.result.current.state.status).toBe('guessing'));
        act(() => hook.result.current.selectLocation({ lat: 48.8, long: 2.3 }));
        await waitFor(() => expect(hook.result.current.state.selectedLocation).toEqual({ lat: 48.8, long: 2.3 }));
        act(() => hook.result.current.submitGuess());
        await waitFor(() => expect(adapter.guess).toHaveBeenCalled());

        hook.rerender({ id: 'challenge-b' });
        expect(guessSignal?.aborted).toBe(true);
        resolveGuess({ score: 4200, duplicate: false });
        await act(async () => {
            await Promise.resolve();
        });

        expect(onStatusChange).not.toHaveBeenCalledWith('challenge-a', 'guessed');
    });

    it('ties an in-flight timeout to the challenge operation', async () => {
        let resolveTimeout!: () => void;
        let timeoutSignal: AbortSignal | undefined;
        const timeoutPromise = new Promise<void>((resolve) => {
            resolveTimeout = resolve;
        });
        const expired = { ...windowFor(-1000), guessExpiresAt: new Date(Date.now() - 1).toISOString() };
        const adapter = adapterWith({
            accept: vi.fn(async () => expired),
            mediaDelivered: vi.fn(async () => expired),
            timeout: vi.fn((_id, signal) => {
                timeoutSignal = signal;
                return timeoutPromise;
            }),
        });
        const hook = renderGame(adapter, 'challenge-a');
        await waitFor(() => expect(adapter.timeout).toHaveBeenCalledWith('challenge-a', expect.any(AbortSignal)));

        hook.rerender({ id: 'challenge-b' });
        expect(timeoutSignal?.aborted).toBe(true);
        resolveTimeout();
        await act(async () => {
            await Promise.resolve();
        });
    });
});
