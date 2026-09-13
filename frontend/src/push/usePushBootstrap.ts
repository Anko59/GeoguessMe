import { useEffect } from 'react';
import { useAuth } from '../context/AuthContext';
import { isPushSupported, syncPushSubscription } from './push';
import { registerServiceWorker } from './serviceWorker';
import { shouldUseWebPush } from '../platform/notifications';

/**
 * Registers the service worker and keeps the push subscription reconciled with
 * the backend. The worker is registered once on mount; the subscription is
 * re-synced whenever the browser rotates it (reported via a service-worker
 * message) and once on each authenticated mount to correct silent drift.
 */
export function usePushBootstrap(): void {
    const { isAuthenticated } = useAuth();

    useEffect(() => {
        if (!shouldUseWebPush()) return;
        void registerServiceWorker();
        if (!isPushSupported()) {
            return;
        }
        const handleMessage = (event: MessageEvent) => {
            if (event.data && event.data.type === 'PUSH_SUBSCRIPTION_CHANGE') {
                void syncPushSubscription();
            }
        };
        navigator.serviceWorker.addEventListener('message', handleMessage);
        return () => {
            navigator.serviceWorker.removeEventListener('message', handleMessage);
        };
    }, []);

    useEffect(() => {
        if (!shouldUseWebPush() || !isAuthenticated || !isPushSupported()) {
            return;
        }
        if (Notification.permission !== 'granted') {
            return;
        }
        void syncPushSubscription();
    }, [isAuthenticated]);
}
