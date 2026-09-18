import { useEffect, useRef, type ReactNode } from 'react';

export default function FeedDialog({
    title,
    onClose,
    busy = false,
    restoreFocus,
    children,
}: {
    title: string;
    onClose: () => void;
    busy?: boolean;
    restoreFocus?: () => void;
    children: ReactNode;
}) {
    const dialog = useRef<HTMLDialogElement>(null);
    useEffect(() => {
        const element = dialog.current;
        const previous = document.activeElement;
        element?.showModal();
        return () => {
            element?.close();
            if (restoreFocus) restoreFocus();
            else if (previous instanceof HTMLElement && previous.isConnected) previous.focus();
        };
    }, [restoreFocus]);
    return (
        <dialog
            ref={dialog}
            className="feed-dialog"
            aria-label={title}
            aria-busy={busy}
            onCancel={(event) => {
                event.preventDefault();
                if (!busy) onClose();
            }}
        >
            <header className="feed-dialog-header">
                <h2>{title}</h2>
                <button className="feed-text-button" aria-label="Close dialog" disabled={busy} onClick={onClose}>
                    Close
                </button>
            </header>
            {children}
        </dialog>
    );
}
