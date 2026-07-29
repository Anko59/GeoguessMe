import { useCallback, useEffect, useRef, useState } from 'react';

export function useCameraDevice() {
    const [facingMode, setFacingMode] = useState<'user' | 'environment'>('user');
    const [hasMultipleCameras, setHasMultipleCameras] = useState(false);
    const facingModeRef = useRef<'user' | 'environment'>('user');
    const restartRef = useRef<() => Promise<void>>(() => Promise.resolve());
    const enumerationRequestRef = useRef(0);

    const refresh = useCallback(async () => {
        const request = ++enumerationRequestRef.current;
        if (!navigator.mediaDevices?.enumerateDevices) return;
        try {
            const devices = await navigator.mediaDevices.enumerateDevices();
            if (request !== enumerationRequestRef.current) return;
            setHasMultipleCameras(devices.filter((d) => d.kind === 'videoinput').length > 1);
        } catch {
            // Camera enumeration is optional; getUserMedia reports the actionable error.
        }
    }, []);

    useEffect(() => {
        void Promise.resolve().then(refresh);
    }, [refresh]);

    const setRestart = useCallback((fn: () => Promise<void>) => {
        restartRef.current = fn;
    }, []);

    const switchCamera = useCallback(() => {
        facingModeRef.current = facingModeRef.current === 'user' ? 'environment' : 'user';
        setFacingMode(facingModeRef.current);
        void restartRef.current();
    }, []);

    return { facingMode, hasMultipleCameras, facingModeRef, refresh, switchCamera, setRestart };
}
