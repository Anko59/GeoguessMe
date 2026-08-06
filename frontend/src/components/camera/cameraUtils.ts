import { useCallback, useEffect, useRef, useState } from 'react';
import api from '../../api';

export const LOCATION_OPTIONS: PositionOptions = {
    enableHighAccuracy: false,
    timeout: 10_000,
    maximumAge: 60_000,
};

export function dataURLToBlob(dataURL: string): Blob {
    const [header, encoded] = dataURL.split(',', 2);
    const binary = atob(encoded);
    const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
    const mimeType = header.match(/^data:([^;]+)/)?.[1] ?? 'image/jpeg';
    return new Blob([bytes], { type: mimeType });
}

export function fitDimensions(width: number, height: number): { width: number; height: number } {
    const maxDimension = 2048;
    const scale = Math.min(1, maxDimension / Math.max(width, height));
    return { width: Math.max(1, Math.round(width * scale)), height: Math.max(1, Math.round(height * scale)) };
}

export function isFilterableImageType(mimeType: string): boolean {
    return FILTERABLE_IMAGE_TYPES.has(mimeType.toLowerCase());
}

export function getCurrentPosition(options: PositionOptions = LOCATION_OPTIONS): Promise<GeolocationPosition> {
    return new Promise((resolve, reject) => {
        if (!navigator.geolocation) return reject(new Error('Geolocation is not supported by your browser'));
        navigator.geolocation.getCurrentPosition(resolve, reject, options);
    });
}

export async function uploadPhoto(
    blob: Blob,
    filename: string,
    groupIDs: string[],
    position: GeolocationPosition,
    hideLocation = false,
): Promise<void> {
    const formData = new FormData();
    formData.append('photo', blob, filename);
    for (const groupID of groupIDs) {
        formData.append('group_ids', groupID);
    }
    formData.append('hide_location', hideLocation ? 'true' : 'false');
    formData.append('lat', position.coords.latitude.toString());
    formData.append('long', position.coords.longitude.toString());
    await api.post('/photo/upload', formData);
}

const FILTERABLE_IMAGE_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp']);

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
