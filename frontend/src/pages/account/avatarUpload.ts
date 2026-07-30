export const AVATAR_MAX_BYTES = 25 * 1024 * 1024;

const supportedTypes = new Set(['image/jpeg', 'image/png', 'image/webp']);
const supportedExtensions = /\.(?:jpe?g|png|webp)$/i;
const heicTypes = new Set(['image/heic', 'image/heif', 'image/heic-sequence', 'image/heif-sequence']);
const heicExtensions = /\.(?:heic|heif)$/i;

export function isHEICFile(file: File): boolean {
    return heicTypes.has(file.type.toLowerCase()) || heicExtensions.test(file.name);
}

function isSupportedImageFile(file: File): boolean {
    return supportedTypes.has(file.type.toLowerCase()) || supportedExtensions.test(file.name);
}

export function validateAvatarFile(file: File): string | null {
    if (file.size > AVATAR_MAX_BYTES) {
        return 'This photo is too large. Choose an image smaller than 25 MiB.';
    }
    if (!isSupportedImageFile(file) && !isHEICFile(file)) {
        return 'This photo format is not supported. Choose a JPG, PNG, WebP, or HEIC/HEIF image.';
    }
    return null;
}

/** Convert HEIC/HEIF locally so the backend only receives a safe web image. */
export async function prepareAvatarFile(file: File): Promise<File> {
    if (!isHEICFile(file)) return file;

    try {
        const { default: convert } = await import('heic2any');
        const converted = await convert({ blob: file, toType: 'image/jpeg', quality: 0.9 });
        const blob = Array.isArray(converted) ? converted[0] : converted;
        if (!(blob instanceof Blob)) throw new Error('HEIC conversion returned no image');
        const name = file.name.replace(/\.[^.]+$/, '') + '.jpg';
        const result = new File([blob], name, { type: 'image/jpeg', lastModified: file.lastModified });
        if (result.size > AVATAR_MAX_BYTES) {
            throw new Error('This photo is too large after conversion. Choose a smaller image and try again.');
        }
        return result;
    } catch (error) {
        if (error instanceof Error && error.message.startsWith('This photo is too large')) throw error;
        throw new Error('This HEIC/HEIF photo could not be converted on this device. Save it as JPG and try again.');
    }
}
