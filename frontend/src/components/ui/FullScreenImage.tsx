import { useEffect, useState, type ReactNode } from 'react';
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

    useEffect(() => {
        if (!expanded) return undefined;
        const closeOnEscape = (event: KeyboardEvent) => {
            if (event.key === 'Escape') setExpanded(false);
        };
        window.addEventListener('keydown', closeOnEscape);
        return () => window.removeEventListener('keydown', closeOnEscape);
    }, [expanded]);

    if (!src) return <>{children}</>;

    return (
        <>
            <button
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
