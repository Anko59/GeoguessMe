import { useCallback, useEffect, useState } from 'react';

const DISMISS_KEY = 'geoguessme:pwa-onboarding-dismissed';

/**
 * Captures the browser `beforeinstallprompt` event so the app can show its own
 * install affordance on its own schedule, and tracks whether the app is already
 * running as an installed standalone PWA.
 */
export interface BeforeInstallPromptEvent extends Event {
    prompt: () => Promise<void>;
    userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>;
}

export interface PwaInstallState {
    /** A native install prompt is available (Android Chrome / desktop Chromium). */
    installable: boolean;
    /** The app is running standalone (installed) rather than in a browser tab. */
    installed: boolean;
    /** The user previously dismissed the onboarding banner. */
    dismissed: boolean;
}

export function isStandaloneDisplay(): boolean {
    if (typeof window === 'undefined') {
        return false;
    }
    if (window.matchMedia && window.matchMedia('(display-mode: standalone)').matches) {
        return true;
    }
    // iOS Safari exposes navigator.standalone instead of the display-mode media query.
    const navigatorStandalone = (navigator as Navigator & { standalone?: boolean }).standalone;
    return navigatorStandalone === true;
}

/** Detect iOS Safari specifically, where Add to Home Screen is a manual flow. */
export function isIosSafari(): boolean {
    if (typeof navigator === 'undefined') {
        return false;
    }
    const ua = navigator.userAgent;
    const isIOS = /iphone|ipad|ipod/i.test(ua);
    // Other iOS browsers (Chrome, Firefox) report as Safari but cannot be added
    // to the home screen the same way; only genuine Safari qualifies.
    const isWebkit = /webkit/i.test(ua);
    const isNotChromeOrEdge = !/crios|fxios|edgios/i.test(ua);
    return isIOS && isWebkit && isNotChromeOrEdge;
}

export function readDismissed(): boolean {
    try {
        return localStorage.getItem(DISMISS_KEY) === '1';
    } catch {
        return false;
    }
}

export function usePwaInstall(): PwaInstallState & {
    promptInstall: () => Promise<'accepted' | 'dismissed' | 'unavailable'>;
    dismiss: () => void;
} {
    const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
    const [installed, setInstalled] = useState<boolean>(() => isStandaloneDisplay());
    const [dismissed, setDismissed] = useState<boolean>(() => readDismissed());

    useEffect(() => {
        const onBeforeInstallPrompt = (event: Event) => {
            event.preventDefault();
            setDeferred(event as BeforeInstallPromptEvent);
        };
        const onAppInstalled = () => {
            setDeferred(null);
            setInstalled(true);
        };
        window.addEventListener('beforeinstallprompt', onBeforeInstallPrompt);
        window.addEventListener('appinstalled', onAppInstalled);
        return () => {
            window.removeEventListener('beforeinstallprompt', onBeforeInstallPrompt);
            window.removeEventListener('appinstalled', onAppInstalled);
        };
    }, []);

    const promptInstall = useCallback(async (): Promise<'accepted' | 'dismissed' | 'unavailable'> => {
        if (!deferred) {
            return 'unavailable';
        }
        await deferred.prompt();
        const choice = await deferred.userChoice;
        setDeferred(null);
        if (choice.outcome === 'accepted') {
            setInstalled(true);
        }
        return choice.outcome;
    }, [deferred]);

    const dismiss = useCallback(() => {
        try {
            localStorage.setItem(DISMISS_KEY, '1');
        } catch {
            /* localStorage may be unavailable (private mode); ignore. */
        }
        setDismissed(true);
    }, []);

    return { installable: deferred !== null, installed, dismissed, promptInstall, dismiss };
}
