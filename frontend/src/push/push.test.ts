import { isPushSupported, pushPermissionState, urlB64ToUint8Array } from './push';

describe('urlB64ToUint8Array', () => {
    it('decodes a known base64url value', () => {
        // "Test" in base64url
        const result = urlB64ToUint8Array('VGVzdA');
        expect(result).toEqual(new Uint8Array([84, 101, 115, 116]));
    });

    it('adds padding implicitly', () => {
        // Three chars -> "abc" in base64 without padding
        const result = urlB64ToUint8Array('YWJj');
        expect(result).toEqual(new Uint8Array([97, 98, 99]));
    });
});

describe('isPushSupported', () => {
    it('returns false when serviceWorker is absent (happy-dom default)', () => {
        expect(isPushSupported()).toBe(false);
    });
});

describe('pushPermissionState', () => {
    it('returns default when Notification is missing', () => {
        expect(pushPermissionState()).toBe('unsupported');
    });
});
