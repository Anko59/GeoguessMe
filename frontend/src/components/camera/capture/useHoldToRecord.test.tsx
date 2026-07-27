import { act, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useHoldToRecord } from './useHoldToRecord';

function Shutter({ onHold, onStop, onTap }: { onHold: () => Promise<void>; onStop: () => void; onTap: () => void }) {
    const gesture = useHoldToRecord({ onHold: () => onHold(), onStop, onTap });
    return (
        <button
            onClick={gesture.onClick}
            onPointerDown={gesture.onPointerDown}
            onPointerUp={gesture.onPointerUp}
            onPointerCancel={gesture.onPointerCancel}
        >
            Shutter
        </button>
    );
}

afterEach(() => vi.useRealTimers());

describe('useHoldToRecord', () => {
    it('takes a photo for a normal shutter tap', () => {
        const onHold = vi.fn(async () => undefined);
        const onStop = vi.fn();
        const onTap = vi.fn();
        render(<Shutter onHold={onHold} onStop={onStop} onTap={onTap} />);

        const shutter = screen.getByRole('button', { name: 'Shutter' });
        fireEvent.pointerDown(shutter);
        fireEvent.pointerUp(shutter);
        fireEvent.click(shutter);

        expect(onTap).toHaveBeenCalledOnce();
        expect(onHold).not.toHaveBeenCalled();
        expect(onStop).not.toHaveBeenCalled();
    });

    it('starts recording only after holding, then stops and suppresses the click photo', async () => {
        vi.useFakeTimers();
        const onHold = vi.fn(async () => undefined);
        const onStop = vi.fn();
        const onTap = vi.fn();
        render(<Shutter onHold={onHold} onStop={onStop} onTap={onTap} />);

        const shutter = screen.getByRole('button', { name: 'Shutter' });
        fireEvent.pointerDown(shutter);
        await act(async () => vi.advanceTimersByTime(300));
        fireEvent.pointerUp(shutter);
        fireEvent.click(shutter);

        expect(onHold).toHaveBeenCalledOnce();
        expect(onStop).toHaveBeenCalledOnce();
        expect(onTap).not.toHaveBeenCalled();
    });
});
