import { useEffect, useRef, useState, type ReactNode } from 'react';
import Icon from './Icon';
import './FullScreenImage.css';

interface FullScreenImageProps {
    /** Full-resolution image URL shown in the dialog. When missing, the
     *  thumbnail renders without the expand affordance (e.g. while a custom
     *  avatar is still loading). */
    src?: string;
    alt: string;
    /** The clickable thumbnail — usually an <img> or Avatar. */
    children: ReactNode;
    /** Extra class for the expand button so the thumbnail keeps its sizing. */
    className?: string;
}

/** Opens a full-screen dialog for an image on click, mirroring the challenge
 *  photo dialogs: an overlay with a close button and Escape support. The
 *  expand button is display:contents so it never disturbs the thumbnail's
 *  layout (avatars, headers, cards all keep their own sizing). */
export default function FullScreenImage({ src, alt, children, className = '' }: FullScreenImageProps) {
    const [expanded, setExpanded] = useState(false);
    const toggleRef = useRef<HTMLButtonElement>(null);
    const closeRef = useRef<HTMLButtonElement>(null);

    useEffect(() => {
        if (!expanded) return undefined;
        const previousOverflow = document.body.style.overflow;
        const toggle = toggleRef.current;
        document.body.style.overflow = 'hidden';
        closeRef.current?.focus();
        const handleDialogKey = (event: KeyboardEvent) => {
            if (event.key === 'Escape') {
                setExpanded(false);
            } else if (event.key === 'Tab') {
                // The close button is the dialog's only interactive control.
                // Keep keyboard focus inside the modal until it is dismissed.
                event.preventDefault();
                closeRef.current?.focus();
            }
        };
        window.addEventListener('keydown', handleDialogKey);
        return () => {
            window.removeEventListener('keydown', handleDialogKey);
            document.body.style.overflow = previousOverflow;
            toggle?.focus();
        };
    }, [expanded]);

    if (!src) return <>{children}</>;

    return (
        <>
            <button
                ref={toggleRef}
                type="button"
                className={`fullscreen-toggle ${className}`.trim()}
                onClick={() => setExpanded(true)}
                aria-label={`View ${alt} full screen`}
                title="View full screen"
            >
                {children}
            </button>
            {expanded && (
                <div className="fullscreen-dialog" role="dialog" aria-modal="true" aria-label={`${alt} full screen`}>
                    <button
                        ref={closeRef}
                        type="button"
                        className="fullscreen-dialog-close"
                        onClick={() => setExpanded(false)}
                        aria-label="Close full-screen photo"
                    >
                        <Icon name="close" />
                    </button>
                    <img src={src} alt={alt} className="fullscreen-dialog-photo" />
                </div>
            )}
        </>
    );
}
