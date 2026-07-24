import { useCallback, useEffect, useRef, useState } from 'react';

export function useCameraDevice() {
    const [facingMode, setFacingMode] = useState<'user' | 'environment'>('user');
    const [hasMultipleCameras, setHasMultipleCameras] = useState(false);
    const facingModeRef = useRef<'user' | 'environment'>('user');
    const restartRef = useRef<() => Promise<void>>(() => Promise.resolve());

    useEffect(() => {
        if (!navigator.mediaDevices?.enumerateDevices) return;
        let active = true;
        navigator.mediaDevices
            .enumerateDevices()
            .then((devices) => {
                if (!active) return;
                setHasMultipleCameras(devices.filter((d) => d.kind === 'videoinput').length > 1);
            })
            .catch(() => {});
        return () => {
            active = false;
        };
    }, []);

    const setRestart = useCallback((fn: () => Promise<void>) => {
        restartRef.current = fn;
    }, []);

    const switchCamera = useCallback(() => {
        facingModeRef.current = facingModeRef.current === 'user' ? 'environment' : 'user';
        setFacingMode(facingModeRef.current);
        void restartRef.current();
    }, []);

    return { facingMode, hasMultipleCameras, facingModeRef, switchCamera, setRestart };
}
