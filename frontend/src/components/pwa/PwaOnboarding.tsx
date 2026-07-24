import { useState } from 'react';
import { useAuth } from '../../context/AuthContext';
import {
    isPushSupported,
    pushPermissionState,
    subscribePushNotifications,
    type PushSubscriptionState,
} from '../../push/push';
import { isIosSafari, usePwaInstall } from './usePwaInstall';
import './PwaOnboarding.css';

export default function PwaOnboarding() {
    const auth = useAuth();
    const { installable, installed, dismissed, promptInstall, dismiss } = usePwaInstall();
    const [permission, setPermission] = useState<PushSubscriptionState>(() => pushPermissionState());
    const [subscribing, setSubscribing] = useState(false);
    const [notice, setNotice] = useState<string | null>(null);

    const iosGuide = !installable && !installed && isIosSafari();
    const pushSupported = isPushSupported();
    // iOS only allows Web Push from an installed PWA, so the notifications
    // affordance is shown only once the app is standalone there.
    const canEnableNotifications = pushSupported && (installed || !isIosSafari());
    const showNotifications = canEnableNotifications && permission !== 'granted';

    if (!auth.isAuthenticated || auth.loading || installed || dismissed) {
        return null;
    }
    if (!installable && !iosGuide && !showNotifications) {
        return null;
    }

    const handleEnableNotifications = async () => {
        setSubscribing(true);
        setNotice(null);
        try {
            const subscription = await subscribePushNotifications();
            setPermission(pushPermissionState());
            setNotice(subscription ? 'Notifications enabled.' : 'Notifications were not enabled.');
        } catch {
            setPermission(pushPermissionState());
            setNotice('Unable to enable notifications right now.');
        } finally {
            setSubscribing(false);
        }
    };

    const handleInstall = async () => {
        const outcome = await promptInstall();
        if (outcome !== 'accepted') {
            setNotice(null);
        }
    };

    return (
        <aside className="pwa-onboarding" role="dialog" aria-label="Install GeoGuessMe">
            <button
                type="button"
                className="pwa-onboarding__close"
                aria-label="Dismiss install prompt"
                onClick={dismiss}
            >
                ×
            </button>
            <div className="pwa-onboarding__body">
                {installable && (
                    <div className="pwa-onboarding__row">
                        <div>
                            <h2 className="pwa-onboarding__title">Install GeoGuessMe</h2>
                            <p className="pwa-onboarding__text">
                                Add it to your home screen for a faster, full-screen experience.
                            </p>
                        </div>
                        <button type="button" className="btn btn-primary" onClick={handleInstall}>
                            Install
                        </button>
                    </div>
                )}
                {iosGuide && (
                    <div className="pwa-onboarding__row">
                        <div>
                            <h2 className="pwa-onboarding__title">Add to Home Screen</h2>
                            <ol className="pwa-onboarding__steps">
                                <li>
                                    Tap the <strong>Share</strong> button in Safari&apos;s toolbar.
                                </li>
                                <li>
                                    Choose <strong>Add to Home Screen</strong>.
                                </li>
                                <li>
                                    Tap <strong>Add</strong> to install GeoGuessMe.
                                </li>
                            </ol>
                            <p className="pwa-onboarding__text">
                                Notifications become available after you install the app on iOS.
                            </p>
                        </div>
                    </div>
                )}
                {showNotifications && (
                    <div className="pwa-onboarding__row">
                        <div>
                            <h2 className="pwa-onboarding__title">Turn on notifications</h2>
                            <p className="pwa-onboarding__text">
                                Get a ping when friends post challenges or send messages.
                            </p>
                        </div>
                        <button
                            type="button"
                            className="btn btn-secondary"
                            onClick={handleEnableNotifications}
                            disabled={subscribing || permission === 'denied'}
                        >
                            {permission === 'denied' ? 'Blocked in settings' : subscribing ? 'Enabling…' : 'Enable'}
                        </button>
                    </div>
                )}
                {notice && (
                    <p className="pwa-onboarding__notice" role="status">
                        {notice}
                    </p>
                )}
            </div>
        </aside>
    );
}
