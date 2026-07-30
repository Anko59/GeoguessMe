import { describe, expect, it, vi } from 'vitest';
import { isHEICFile, prepareAvatarFile, validateAvatarFile } from './avatarUpload';

const convert = vi.fn();

vi.mock('heic2any', () => ({ default: convert }));

describe('avatar upload preparation', () => {
    it('accepts web image formats and rejects unknown formats with guidance', () => {
        expect(validateAvatarFile(new File(['jpg'], 'photo.jpg', { type: 'image/jpeg' }))).toBeNull();
        expect(validateAvatarFile(new File(['png'], 'photo.png', { type: 'image/png' }))).toBeNull();
        expect(validateAvatarFile(new File(['gif'], 'photo.gif', { type: 'image/gif' }))).toContain('not supported');
    });

    it('reports files larger than the upload limit before the request starts', () => {
        const file = new File(['photo'], 'photo.jpg', { type: 'image/jpeg' });
        Object.defineProperty(file, 'size', { value: 25 * 1024 * 1024 + 1 });
        expect(validateAvatarFile(file)).toBe('This photo is too large. Choose an image smaller than 25 MiB.');
    });

    it('converts HEIC files locally to JPEG before upload', async () => {
        const source = new File(['heic'], 'photo.HEIC', { type: 'image/heic' });
        const converted = new Blob(['jpeg'], { type: 'image/jpeg' });
        convert.mockResolvedValueOnce(converted);

        expect(isHEICFile(source)).toBe(true);
        const result = await prepareAvatarFile(source);
        expect(convert).toHaveBeenCalledWith({ blob: source, toType: 'image/jpeg', quality: 0.9 });
        expect(result.name).toBe('photo.jpg');
        expect(result.type).toBe('image/jpeg');
    });
});
