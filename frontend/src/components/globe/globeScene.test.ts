import * as THREE from 'three';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
    createGlobeScene,
    earthMinDistance,
    earthTextureURL,
    FALLBACK_EARTH_TEXTURE_URL,
    FALLBACK_MIN_DISTANCE,
    globePosition,
    HIGH_RES_EARTH_TEXTURE_URL,
    HIGH_RES_MIN_DISTANCE,
    earthTextureSize,
} from './globeScene';

const mocks = vi.hoisted(() => ({
    render: vi.fn(),
    dispose: vi.fn(),
    forceContextLoss: vi.fn(),
    disconnect: vi.fn(),
    clearPinOverlay: vi.fn(),
    drawPinArtwork: vi.fn(),
    setPinOverlayTransform: vi.fn(),
    maxTextureSize: 8192,
}));
vi.mock('three', async (importOriginal) => {
    const actual = await importOriginal<typeof import('three')>();
    return {
        ...actual,
        WebGLRenderer: class {
            domElement = document.createElement('canvas');
            capabilities = {
                maxTextureSize: mocks.maxTextureSize,
                getMaxAnisotropy: () => 8,
            };
            setPixelRatio() {}
            setSize() {}
            render = mocks.render;
            dispose = mocks.dispose;
            forceContextLoss = mocks.forceContextLoss;
        },
    };
});

beforeEach(() => {
    vi.clearAllMocks();
    mocks.maxTextureSize = 8192;
    const pinOverlayContext = {
        clearRect: mocks.clearPinOverlay,
        drawImage: mocks.drawPinArtwork,
        setTransform: mocks.setPinOverlayTransform,
    } as unknown as CanvasRenderingContext2D;
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockImplementation((contextID) =>
        contextID === '2d' ? pinOverlayContext : null,
    );
    vi.stubGlobal(
        'ResizeObserver',
        class {
            observe() {}
            disconnect = mocks.disconnect;
        },
    );
});
afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('Earth scene', () => {
    it('selects the high-resolution asset only when the GPU can upload it', () => {
        expect(earthTextureURL(8192)).toBe(HIGH_RES_EARTH_TEXTURE_URL);
        expect(earthTextureURL(16384)).toBe(HIGH_RES_EARTH_TEXTURE_URL);
        expect(earthTextureURL(8192, true)).toBe(FALLBACK_EARTH_TEXTURE_URL);
        expect(earthTextureURL(4096)).toBe(FALLBACK_EARTH_TEXTURE_URL);
        expect(earthTextureSize(8192)).toBe(8192);
        expect(earthTextureSize(8192, true)).toBe(2048);
        expect(earthTextureSize(4096)).toBe(2048);
        expect(earthMinDistance(8192)).toBe(HIGH_RES_MIN_DISTANCE);
        expect(earthMinDistance(4096)).toBe(FALLBACK_MIN_DISTANCE);
    });

    it('configures mipmaps and anisotropic filtering for the high-resolution texture', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        let requestedURL = '';
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((url, onLoad) => {
            requestedURL = String(url);
            onLoad?.(texture);
            return texture;
        });
        const host = document.createElement('div');
        Object.defineProperty(host, 'clientWidth', { value: 1000 });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        expect(requestedURL).toBe(HIGH_RES_EARTH_TEXTURE_URL);
        expect(texture.minFilter).toBe(THREE.LinearMipmapLinearFilter);
        expect(texture.magFilter).toBe(THREE.LinearFilter);
        expect(texture.anisotropy).toBe(4);
        expect(globe.controls.minDistance).toBe(HIGH_RES_MIN_DISTANCE);
        globe.dispose();
    });

    it('keeps the bundled texture and conservative zoom on smaller WebGL limits', () => {
        mocks.maxTextureSize = 4096;
        const texture = new THREE.Texture<HTMLImageElement>();
        let requestedURL = '';
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((url) => {
            requestedURL = String(url);
            return texture;
        });
        const globe = createGlobeScene(document.createElement('div'), vi.fn(), vi.fn());
        expect(requestedURL).toBe(FALLBACK_EARTH_TEXTURE_URL);
        expect(globe.controls.minDistance).toBe(FALLBACK_MIN_DISTANCE);
        globe.dispose();
    });

    it('uses the mobile texture and tile budget for a 700px coarse-pointer device', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        let requestedURL = '';
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((url) => {
            requestedURL = String(url);
            return texture;
        });
        vi.stubGlobal('matchMedia', (query: string) => ({ matches: query === '(pointer: coarse)' }));
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 700 }, clientHeight: { value: 500 } });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        expect(requestedURL).toBe(FALLBACK_EARTH_TEXTURE_URL);
        globe.dispose();
    });

    it('keeps touch navigation surface-tracked, pole-safe and pinch-anchored', () => {
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        const controls = globe.controls;
        expect(controls.enablePan).toBe(false);
        expect(controls.zoomToCursor).toBe(false);
        expect(controls.touches).toEqual({ ONE: THREE.TOUCH.ROTATE, TWO: THREE.TOUCH.DOLLY_PAN });
        expect(controls.minPolarAngle).toBeCloseTo(0.05);
        expect(controls.maxPolarAngle).toBeCloseTo(Math.PI - 0.05);
        const surfaceSpeed = (distance: number) =>
            (Math.tan(THREE.MathUtils.degToRad(21)) * Math.sqrt(distance * distance - 1)) / Math.PI;
        expect(controls.rotateSpeed).toBeCloseTo(surfaceSpeed(3.5), 3);
        globe.zoom(0.5);
        expect(controls.rotateSpeed).toBeCloseTo(surfaceSpeed(3.5) / 2, 3);
        expect(controls.rotateSpeed).toBeLessThan(surfaceSpeed(3.5));
        globe.dispose();
    });

    it('damps gesture inertia only while a gesture is in flight', () => {
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(new THREE.Texture<HTMLImageElement>());
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        document.body.append(host);
        const globe = createGlobeScene(host, vi.fn(), vi.fn());
        const canvas = host.querySelector('canvas')!;
        canvas.setPointerCapture = vi.fn();
        canvas.releasePointerCapture = vi.fn();
        canvas.dispatchEvent(
            new PointerEvent('pointerdown', { clientX: 10, clientY: 10, pointerId: 1, pointerType: 'touch' }),
        );
        expect(globe.controls.enableDamping).toBe(true);
        globe.dispose();
        expect(host.children).toHaveLength(0);
        host.remove();
    });

    it('releases partially initialized resources if observing the canvas fails', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockReturnValue(texture);
        const releaseTexture = vi.spyOn(texture, 'dispose');
        vi.stubGlobal(
            'ResizeObserver',
            class {
                observe() {
                    throw new Error('observe failed');
                }
                disconnect = mocks.disconnect;
            },
        );
        const host = document.createElement('div');
        expect(() => createGlobeScene(host, vi.fn(), vi.fn())).toThrow('observe failed');
        expect(host.children).toHaveLength(0);
        expect(releaseTexture).toHaveBeenCalledOnce();
        expect(mocks.disconnect).toHaveBeenCalledOnce();
        expect(mocks.dispose).toHaveBeenCalledOnce();
    });

    it('cleans up the renderer when the pin overlay context is unavailable', () => {
        vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null);
        const host = document.createElement('div');

        expect(() => createGlobeScene(host, vi.fn(), vi.fn())).toThrow('Canvas 2D is unavailable');
        expect(host.children).toHaveLength(0);
        expect(mocks.dispose).toHaveBeenCalledOnce();
        expect(mocks.forceContextLoss).toHaveBeenCalledOnce();
    });

    it('ignores a texture completing after the globe is closed', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        let complete: ((texture: THREE.Texture<HTMLImageElement>) => void) | undefined;
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((_url, onLoad) => {
            complete = onLoad;
            return texture;
        });
        const onReady = vi.fn();
        const globe = createGlobeScene(document.createElement('div'), vi.fn(), vi.fn(), onReady);
        globe.dispose();
        mocks.render.mockClear();
        complete?.(texture);
        expect(mocks.render).not.toHaveBeenCalled();
        expect(onReady).not.toHaveBeenCalled();
        globe.dispose();
        expect(mocks.dispose).toHaveBeenCalledOnce();
    });

    it('announces readiness once the Earth texture has decoded and rendered', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        let complete: ((texture: THREE.Texture<HTMLImageElement>) => void) | undefined;
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((_url, onLoad) => {
            complete = onLoad;
            return texture;
        });
        const onReady = vi.fn();
        const globe = createGlobeScene(document.createElement('div'), vi.fn(), vi.fn(), onReady);
        expect(onReady).not.toHaveBeenCalled();
        mocks.render.mockClear();
        complete?.(texture);
        expect(onReady).toHaveBeenCalledOnce();
        expect(mocks.render).toHaveBeenCalled();
        globe.dispose();
    });

    it('does not announce readiness when the Earth texture fails to load', () => {
        const texture = new THREE.Texture<HTMLImageElement>();
        let fail: ((error: unknown) => void) | undefined;
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((_url, _onLoad, _onProgress, onError) => {
            fail = onError;
            return texture;
        });
        const onError = vi.fn();
        const onReady = vi.fn();
        const globe = createGlobeScene(document.createElement('div'), vi.fn(), onError, onReady);
        fail?.(new Error('texture failed'));
        expect(onError).toHaveBeenCalledOnce();
        expect(onReady).not.toHaveBeenCalled();
        globe.dispose();
    });

    it('requests sharp detail on zoom and keeps local Earth navigation working when tiles fail', async () => {
        vi.useFakeTimers();
        const texture = new THREE.Texture<HTMLImageElement>();
        vi.spyOn(THREE.TextureLoader.prototype, 'load').mockImplementation((_url, onLoad) => {
            onLoad?.(texture);
            return texture;
        });
        const fetchMock = vi.fn().mockRejectedValue(new TypeError('offline'));
        vi.stubGlobal('fetch', fetchMock);
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 500 }, clientHeight: { value: 400 } });
        document.body.append(host);
        const onError = vi.fn();
        const onReady = vi.fn();
        const onFallback = vi.fn();
        const globe = createGlobeScene(host, vi.fn(), onError, onReady, onFallback);
        expect(onReady).toHaveBeenCalledOnce();
        globe.zoom(0.01);
        await vi.advanceTimersByTimeAsync(120);
        await Promise.resolve();
        await Promise.resolve();
        expect(fetchMock).toHaveBeenCalled();
        expect(fetchMock.mock.calls[0][0]).toMatch(/^https:\/\/tile\.openstreetmap\.org\/\d+\/\d+\/\d+\.png$/);
        expect(onFallback).toHaveBeenCalledWith(true);
        expect(onError).not.toHaveBeenCalled();
        expect(globe.controls.enabled).toBe(true);
        globe.zoom(1_000_000_000);
        expect((mocks.render.mock.lastCall?.[1] as THREE.PerspectiveCamera).zoom).toBe(1);
        globe.dispose();
        expect(host.children).toHaveLength(0);
        host.remove();
    });
    it('aligns Greenwich, poles and the date line to the Earth texture', () => {
        expect(globePosition(0, 0).x).toBeCloseTo(1);
        expect(globePosition(90, 0).y).toBeCloseTo(1);
        expect(globePosition(-90, 0).y).toBeCloseTo(-1);
        expect(globePosition(0, 90).z).toBeCloseTo(-1);
        expect(globePosition(0, -90).z).toBeCloseTo(1);
        expect(globePosition(0, 180).x).toBeCloseTo(-1);
        expect(globePosition(48.8566, 2.3522, 2).length()).toBeCloseTo(2);
    });
});
