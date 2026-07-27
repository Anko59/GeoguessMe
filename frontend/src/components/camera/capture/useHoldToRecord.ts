import { useCallback, useEffect, useRef } from 'react';

const HOLD_TO_RECORD_MS = 300;

interface HoldToRecordOptions {
    onHold: (isStillPressed: () => boolean) => Promise<void>;
    onStop: () => void;
    onTap: () => void;
}

export function useHoldToRecord({ onHold, onStop, onTap }: HoldToRecordOptions) {
    const timerRef = useRef<number | null>(null);
    const pressingRef = useRef(false);
    const suppressClickRef = useRef(false);

    const clearHold = useCallback(() => {
        if (timerRef.current !== null) window.clearTimeout(timerRef.current);
        timerRef.current = null;
    }, []);

    const onPointerDown = useCallback(() => {
        pressingRef.current = true;
        suppressClickRef.current = false;
        timerRef.current = window.setTimeout(() => {
            timerRef.current = null;
            suppressClickRef.current = true;
            void onHold(() => pressingRef.current);
        }, HOLD_TO_RECORD_MS);
    }, [onHold]);

    const onPointerUp = useCallback(() => {
        pressingRef.current = false;
        clearHold();
        if (suppressClickRef.current) onStop();
    }, [clearHold, onStop]);

    const onPointerCancel = useCallback(() => {
        pressingRef.current = false;
        clearHold();
        suppressClickRef.current = false;
        onStop();
    }, [clearHold, onStop]);

    const onClick = useCallback(() => {
        if (suppressClickRef.current) {
            suppressClickRef.current = false;
            return;
        }
        onTap();
    }, [onTap]);

    useEffect(
        () => () => {
            pressingRef.current = false;
            clearHold();
            onStop();
        },
        [clearHold, onStop],
    );

    return { onClick, onPointerDown, onPointerUp, onPointerCancel };
}
