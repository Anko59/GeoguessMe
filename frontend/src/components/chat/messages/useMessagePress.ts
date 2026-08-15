import { useCallback, useEffect, useRef } from 'react';

/** How long a touch must be held before the reply/react panel opens. This is
 *  the deliberate long press that replaces tap-to-open on touch devices. */
export const LONG_PRESS_MS = 1000;

/** Horizontal drift that outruns the vertical drift reveals the actions like
 *  a long press (swipe-to-reveal). */
const SWIPE_REVEAL_PX = 28;

/** Vertical drift that outruns the horizontal drift cancels the press, so
 *  scrolling the list never reveals the actions. */
const SWIPE_CANCEL_PX = 20;

interface PressState {
    messageID: string;
    startX: number;
    startY: number;
    timer: number;
}

/** The pointer/focus handlers a message row binds to its container. */
export interface MessagePressHandlers {
    onPointerDown: (event: React.PointerEvent<HTMLElement>) => void;
    onPointerMove: (event: React.PointerEvent<HTMLElement>) => void;
    onPointerUp: () => void;
    onPointerCancel: () => void;
    onFocus: () => void;
}

export interface UseMessagePressOptions {
    /** A plain tap landing on a row: dismisses actions shown on other rows. */
    onTapDown: (messageID: string) => void;
    /** A long press, a swipe, or keyboard focus reveals the row's actions. */
    onReveal: (messageID: string) => void;
}

/**
 * Long-press and swipe-to-reveal interactions for a chat message row.
 *
 * - A deliberate LONG_PRESS_MS hold (or a horizontal swipe past SWIPE_REVEAL_PX
 *   that outruns the vertical drift) reveals the message actions; a plain tap
 *   or a short hold never does.
 * - Vertical drift past SWIPE_CANCEL_PX cancels the press, so scrolling the
 *   list never reveals actions.
 * - The reveal timer is cleared on pointer end/cancel AND on unmount, so a row
 *   can never fire an interaction after it leaves the screen.
 * - Hardware-keyboard users reveal the actions by focusing the row (the row is
 *   tabbable), so the panel is reachable without hover or touch.
 *
 * Returns a function that binds the handlers for one message id. Handlers are
 * recreated per render, so they close over the latest callbacks; the press
 * state itself lives in the hook's refs and is only touched by events and the
 * unmount cleanup, never during render.
 */
export function useMessagePress({
    onTapDown,
    onReveal,
}: UseMessagePressOptions): (messageID: string, enabled: boolean) => MessagePressHandlers {
    const pressRef = useRef<PressState | null>(null);

    const clearPress = useCallback(() => {
        if (pressRef.current) window.clearTimeout(pressRef.current.timer);
        pressRef.current = null;
    }, []);

    // Never let a reveal timer fire after the row unmounts.
    useEffect(() => clearPress, [clearPress]);

    return useCallback(
        (messageID: string, enabled: boolean): MessagePressHandlers => ({
            onPointerDown: (event) => {
                if (!event.isPrimary || (event.target as HTMLElement).closest('button')) return;
                onTapDown(messageID);
                clearPress();
                const timer = window.setTimeout(() => onReveal(messageID), LONG_PRESS_MS);
                pressRef.current = { messageID, startX: event.clientX, startY: event.clientY, timer };
            },
            onPointerMove: (event) => {
                const press = pressRef.current;
                if (!press || press.messageID !== messageID) return;
                const horizontal = Math.abs(event.clientX - press.startX);
                const vertical = Math.abs(event.clientY - press.startY);
                if (vertical > SWIPE_CANCEL_PX && vertical > horizontal) {
                    clearPress();
                    return;
                }
                if (horizontal > SWIPE_REVEAL_PX && horizontal > vertical) {
                    onReveal(messageID);
                    clearPress();
                }
            },
            onPointerUp: clearPress,
            onPointerCancel: clearPress,
            onFocus: () => {
                if (enabled) onReveal(messageID);
            },
        }),
        [clearPress, onTapDown, onReveal],
    );
}
