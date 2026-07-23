import api from '../../api';

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

export function getCurrentPosition(): Promise<GeolocationPosition> {
    return new Promise((resolve, reject) => {
        if (!navigator.geolocation) return reject(new Error('Geolocation is not supported by your browser'));
        navigator.geolocation.getCurrentPosition(resolve, reject);
    });
}

export async function uploadPhoto(blob: Blob, filename: string, groupID: string): Promise<void> {
    const position = await getCurrentPosition();
    const formData = new FormData();
    formData.append('photo', blob, filename);
    formData.append('group_id', groupID);
    formData.append('lat', position.coords.latitude.toString());
    formData.append('long', position.coords.longitude.toString());
    await api.post('/photo/upload', formData);
}

const FILTERABLE_IMAGE_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp']);
