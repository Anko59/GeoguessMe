import { act, fireEvent, render, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { LONG_PRESS_MS, useMessagePress } from './useMessagePress';

/** A row-like host so the hook's handlers run inside a mounted component. */
function RowHarness({
    messageID,
    enabled,
    onTapDown,
    onReveal,
}: {
    messageID: string;
    enabled: boolean;
    onTapDown: (messageID: string) => void;
    onReveal: (messageID: string) => void;
}) {
    const bindPress = useMessagePress({ onTapDown, onReveal });
    const handlers = bindPress(messageID, enabled);
    return (
        <div
            data-testid="row"
            onPointerDown={(event) => handlers.onPointerDown(event)}
            onPointerMove={(event) => handlers.onPointerMove(event)}
            onPointerUp={handlers.onPointerUp}
            onPointerCancel={handlers.onPointerCancel}
            onFocus={handlers.onFocus}
        >
            row
        </div>
    );
}

const pointerEvent = (x: number, y: number, overrides: Partial<React.PointerEvent<HTMLElement>> = {}) =>
    ({
        isPrimary: true,
        clientX: x,
        clientY: y,
        target: document.createElement('div'),
        ...overrides,
    }) as unknown as React.PointerEvent<HTMLElement>;

beforeEach(() => {
    vi.useFakeTimers();
});

afterEach(() => {
    vi.useRealTimers();
});

describe('useMessagePress', () => {
    it('reveals the actions only after the full long-press duration', () => {
        const reveal = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: vi.fn(), onReveal: reveal }));

        result.current('message-1', true).onPointerDown(pointerEvent(10, 10));
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS - 1);
        });
        expect(reveal).not.toHaveBeenCalled();
        act(() => {
            vi.advanceTimersByTime(1);
        });
        expect(reveal).toHaveBeenCalledExactlyOnceWith('message-1');
    });

    it('never reveals on a plain tap and dismisses other rows on tap-down', () => {
        const reveal = vi.fn();
        const tapDown = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: tapDown, onReveal: reveal }));

        const handlers = result.current('message-1', true);
        handlers.onPointerDown(pointerEvent(10, 10));
        handlers.onPointerUp();
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(reveal).not.toHaveBeenCalled();
        expect(tapDown).toHaveBeenCalledExactlyOnceWith('message-1');
    });

    it('ignores pointer events that start on a button inside the row', () => {
        const reveal = vi.fn();
        const tapDown = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: tapDown, onReveal: reveal }));

        const button = document.createElement('button');
        result.current('message-1', true).onPointerDown(pointerEvent(10, 10, { target: button }));
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(tapDown).not.toHaveBeenCalled();
        expect(reveal).not.toHaveBeenCalled();
    });

    it('cancels the press on vertical scroll drift', () => {
        const reveal = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: vi.fn(), onReveal: reveal }));

        const handlers = result.current('message-1', true);
        handlers.onPointerDown(pointerEvent(10, 10));
        handlers.onPointerMove(pointerEvent(12, 60));
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(reveal).not.toHaveBeenCalled();
    });

    it('reveals the actions on a horizontal swipe that outruns the vertical drift', () => {
        const reveal = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: vi.fn(), onReveal: reveal }));

        const handlers = result.current('message-1', true);
        handlers.onPointerDown(pointerEvent(10, 10));
        handlers.onPointerMove(pointerEvent(50, 14));
        expect(reveal).toHaveBeenCalledExactlyOnceWith('message-1');
        // The press is consumed, so a subsequent hold cannot fire again.
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(reveal).toHaveBeenCalledTimes(1);
    });

    it('ignores pointer movement for a different row', () => {
        const reveal = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: vi.fn(), onReveal: reveal }));

        const first = result.current('message-1', true);
        const second = result.current('message-2', true);
        first.onPointerDown(pointerEvent(10, 10));
        // The move belongs to another row's binding: no reveal for message-1.
        second.onPointerMove(pointerEvent(60, 10));
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(reveal).toHaveBeenCalledExactlyOnceWith('message-1');
    });

    it('reveals actions on keyboard focus for enabled rows only', () => {
        const reveal = vi.fn();
        const { result } = renderHook(() => useMessagePress({ onTapDown: vi.fn(), onReveal: reveal }));

        result.current('message-1', true).onFocus();
        expect(reveal).toHaveBeenCalledExactlyOnceWith('message-1');
        result.current('message-2', false).onFocus();
        expect(reveal).toHaveBeenCalledTimes(1);
    });

    it('clears the reveal timer when the row unmounts', () => {
        const reveal = vi.fn();
        const { unmount } = render(<RowHarness messageID="message-1" enabled onTapDown={vi.fn()} onReveal={reveal} />);
        const row = document.querySelector('[data-testid="row"]') as HTMLElement;
        fireEvent.pointerDown(row, { isPrimary: true, clientX: 10, clientY: 10 });
        unmount();
        act(() => {
            vi.advanceTimersByTime(LONG_PRESS_MS);
        });
        expect(reveal).not.toHaveBeenCalled();
    });

    it('exposes the long-press duration as a constant consumers can assert against', () => {
        expect(LONG_PRESS_MS).toBe(1000);
    });
});
