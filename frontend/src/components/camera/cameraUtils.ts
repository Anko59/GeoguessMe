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
const FILTERABLE_IMAGE_TYPES = new Set(['image/jpeg', 'image/png', 'image/webp']);
