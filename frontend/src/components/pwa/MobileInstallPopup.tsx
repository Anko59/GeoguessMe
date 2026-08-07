import { useState } from 'react';
import { isIosSafari, isStandaloneDisplay, usePwaInstall } from './usePwaInstall';
import Icon from '../ui/Icon';
import './MobileInstallPopup.css';

export default function MobileInstallPopup() {
    const { installable, promptInstall } = usePwaInstall();
    const [visible, setVisible] = useState(() => {
        if (isStandaloneDisplay()) return false;
        try {
            return sessionStorage.getItem('geoguessme:pwa-popup-dismissed') !== '1';
        } catch {
            return true;
        }
    });

    if (!visible) return null;

    const isMobile =
        typeof window !== 'undefined' && window.matchMedia && window.matchMedia('(pointer: coarse)').matches;
    if (!isMobile) return null;

    const dismiss = () => {
        try {
            sessionStorage.setItem('geoguessme:pwa-popup-dismissed', '1');
        } catch {
            /* ignore */
        }
        setVisible(false);
    };

    const iosGuide = isIosSafari();
    const handleInstall = async () => {
        const outcome = await promptInstall();
        if (outcome !== 'unavailable') {
            dismiss();
        }
    };

    return (
        <div className="mobile-install-popup" role="dialog" aria-label="Install GeoGuessMe">
            <div className="mobile-install-popup__card">
                <button type="button" className="mobile-install-popup__close" aria-label="Close" onClick={dismiss}>
                    <Icon name="close" />
                </button>
                <p className="mobile-install-popup__text">
                    {iosGuide
                        ? 'Install GeoGuessMe for a full-screen experience and push notifications.'
                        : 'Add GeoGuessMe to your home screen for a native app experience.'}
                </p>
                {iosGuide ? (
                    <ol className="mobile-install-popup__steps">
                        <li>
                            Tap <strong>Share</strong> in Safari
                        </li>
                        <li>
                            Choose <strong>Add to Home Screen</strong>
                        </li>
                        <li>
                            Tap <strong>Add</strong>
                        </li>
                    </ol>
                ) : installable ? (
                    <button type="button" className="btn btn-primary" onClick={() => void handleInstall()}>
                        Install app
                    </button>
                ) : (
                    <button type="button" className="btn btn-primary" onClick={dismiss}>
                        Got it
                    </button>
                )}
            </div>
        </div>
    );
}
