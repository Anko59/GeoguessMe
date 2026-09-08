import { useEffect } from 'react';
import { App as NativeApp } from '@capacitor/app';
import { StatusBar, Style } from '@capacitor/status-bar';
import { useLocation, useNavigate } from 'react-router-dom';
import { currentRuntime, isNativeRuntime } from './runtime';

function routeFromAppURL(value: string): string | null {
    try {
        const url = new URL(value);
        if (url.protocol === 'https:' && url.hostname !== 'geoguessme.com' && url.hostname !== 'www.geoguessme.com') {
            return null;
        }
        if (url.protocol !== 'https:' && url.protocol !== 'geoguessme:') return null;
        const path = url.protocol === 'geoguessme:' ? `/${url.hostname}${url.pathname}` : url.pathname;
        return `${path.replace(/\/+/g, '/')}${url.search}${url.hash}`;
    } catch {
        return null;
    }
}

/** Owns native-only app lifecycle behavior so router components remain
 * platform-neutral: deep links, system back, and status-bar appearance. */
export function useNativeAppLifecycle(): void {
    const navigate = useNavigate();
    const location = useLocation();

    useEffect(() => {
        if (!isNativeRuntime()) return;
        document.documentElement.classList.add('native-runtime');
        void StatusBar.setStyle({ style: Style.Dark });
        if (currentRuntime() === 'android') {
            void StatusBar.setBackgroundColor({ color: '#f7f5ff' });
        }

        const listeners = Promise.all([
            NativeApp.addListener('appUrlOpen', ({ url }) => {
                const route = routeFromAppURL(url);
                if (route) navigate(route);
            }),
            NativeApp.addListener('backButton', () => {
                if (location.pathname === '/' || location.pathname === '/groups') {
                    void NativeApp.exitApp();
                    return;
                }
                navigate(-1);
            }),
        ]);
        return () => {
            document.documentElement.classList.remove('native-runtime');
            void listeners.then((handles) => handles.forEach((handle) => void handle.remove()));
        };
    }, [location.pathname, navigate]);
}

export { routeFromAppURL };
