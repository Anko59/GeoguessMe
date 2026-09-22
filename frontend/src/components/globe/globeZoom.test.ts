import * as THREE from 'three';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createGlobeDetailLayer, tileLatitudeAtRow, visibleTiles } from './globeDetailTiles';
import {
    detailSourceForCamera,
    maxUsefulGlobeZoom,
    OSM_TILE_MAX_PIXELS_PER_DEGREE,
    isGlobeMobile,
    tilePixelsPerDegree,
    visibleSurfaceWidthDegrees,
} from './globeZoom';
import { globePosition } from './globeScene';

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<T>((resolvePromise, rejectPromise) => {
        resolve = resolvePromise;
        reject = rejectPromise;
    });
    return { promise, resolve, reject };
}

function tileResponse(): Response {
    return { ok: true, status: 200, blob: async () => new Blob(['tile']) } as Response;
}

function meshWaiter() {
    let layer: THREE.Group | undefined;
    const waiters: { predicate: () => boolean; resolve: () => void }[] = [];
    const meshes = (predicate: (mesh: THREE.Mesh) => boolean = () => true) =>
        layer?.children.filter((child): child is THREE.Mesh => child instanceof THREE.Mesh && predicate(child)) ?? [];
    const render = vi.fn(() => {
        for (let index = waiters.length - 1; index >= 0; index -= 1) {
            if (!waiters[index].predicate()) continue;
            waiters[index].resolve();
            waiters.splice(index, 1);
        }
    });
    return {
        meshes,
        render,
        setLayer(value: THREE.Group) {
            layer = value;
        },
        waitFor(predicate: () => boolean) {
            if (predicate()) return Promise.resolve();
            return new Promise<void>((resolve) => waiters.push({ predicate, resolve }));
        },
    };
}

function meshRadius(mesh: THREE.Mesh): number {
    const position = mesh.geometry.getAttribute('position');
    return new THREE.Vector3(position.getX(0), position.getY(0), position.getZ(0)).length();
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
});

function cameraAt(latitude: number, longitude: number, aspect: number) {
    const camera = new THREE.PerspectiveCamera(42, aspect, 0.1, 50);
    const lat = THREE.MathUtils.degToRad(latitude);
    const lon = THREE.MathUtils.degToRad(longitude);
    camera.position.set(3.5 * Math.cos(lat) * Math.cos(lon), 3.5 * Math.sin(lat), -3.5 * Math.cos(lat) * Math.sin(lon));
    camera.lookAt(0, 0, 0);
    camera.updateProjectionMatrix();
    camera.updateMatrixWorld(true);
    return camera;
}

describe('globe detail resolution', () => {
    it('uses Mercator row spacing for OSM geometry and linear latitude for NASA', () => {
        const osmTile = { provider: 'osm' as const, level: 3, row: 0, column: 0 };
        const expectedMercatorLatitude = THREE.MathUtils.radToDeg(Math.atan(Math.sinh(Math.PI * (1 - 1 / 8))));
        const incorrectLinearLatitude = (85.05112878 + 79.17133464) / 2;
        expect(tileLatitudeAtRow(osmTile, 0.5)).toBeCloseTo(expectedMercatorLatitude, 6);
        expect(Math.abs(tileLatitudeAtRow(osmTile, 0.5) - incorrectLinearLatitude)).toBeGreaterThan(0.5);

        const nasaTile = { provider: 'nasa' as const, level: 0, row: 0, column: 0 };
        expect(tileLatitudeAtRow(nasaTile, 0.5)).toBe(0);
    });

    it('shares the scene mobile budget for narrow and coarse-pointer devices', () => {
        expect(isGlobeMobile(700, true)).toBe(true);
        expect(isGlobeMobile(770, true)).toBe(true);
        expect(isGlobeMobile(770, false)).toBe(false);
        expect(isGlobeMobile(768, false)).toBe(true);
    });
    it('moves from the local texture to NASA tiles and then to city-scale OSM tiles', () => {
        const camera = cameraAt(20, 0, 1.6);
        expect(detailSourceForCamera(camera, 1280, 2, 8192)).toBeNull();

        camera.zoom = 4;
        camera.updateProjectionMatrix();
        const nasaSource = detailSourceForCamera(camera, 1280, 2, 8192);
        expect(nasaSource?.provider).toBe('nasa');
        expect(tilePixelsPerDegree(nasaSource!.level)).toBeGreaterThanOrEqual(
            ((1280 * 2) / visibleSurfaceWidthDegrees(camera)) * 1.2,
        );

        camera.zoom = 32;
        camera.updateProjectionMatrix();
        expect(detailSourceForCamera(camera, 1280, 2, 8192)?.provider).toBe('osm');
    });

    it('caps desktop and mobile zoom at the available z19 source detail', () => {
        const desktop = cameraAt(20, 0, 1.6);
        const desktopMax = maxUsefulGlobeZoom(desktop, 1280, 2);
        desktop.zoom = desktopMax;
        desktop.updateProjectionMatrix();
        const desktopWidth = visibleSurfaceWidthDegrees(desktop);
        const desktopPixelsPerDegree = (1280 * 2) / desktopWidth;
        expect(desktopWidth).toBeLessThan(0.009);
        expect(desktopPixelsPerDegree * 1.2).toBeCloseTo(OSM_TILE_MAX_PIXELS_PER_DEGREE, 2);
        expect(detailSourceForCamera(desktop, 1280, 2, 8192)).toEqual({ provider: 'osm', level: 19 });

        const mobile = cameraAt(20, 0, 390 / 844);
        const mobileMax = maxUsefulGlobeZoom(mobile, 390, 2);
        mobile.zoom = mobileMax;
        mobile.updateProjectionMatrix();
        const mobileWidth = visibleSurfaceWidthDegrees(mobile);
        expect(mobileWidth).toBeLessThan(0.003);
        expect((390 * 2) / mobileWidth).toBeGreaterThan(300_000);
        expect(detailSourceForCamera(mobile, 390, 2, 2048)).toEqual({ provider: 'osm', level: 19 });
    });

    it('wraps OSM tiles across the date line and leaves polar caps on bundled imagery', () => {
        const dateLine = cameraAt(10, 179.999, 1.6);
        dateLine.zoom = maxUsefulGlobeZoom(dateLine, 1280, 2);
        dateLine.updateProjectionMatrix();
        const dateLineTiles = visibleTiles(dateLine, 'osm', 19, 64);
        expect(dateLineTiles.length).toBeGreaterThan(0);
        expect(dateLineTiles.some((tile) => tile.column === 0)).toBe(true);
        expect(dateLineTiles.some((tile) => tile.column === 2 ** 19 - 1)).toBe(true);

        const polar = cameraAt(88, 0, 1.6);
        polar.zoom = maxUsefulGlobeZoom(polar, 1280, 2);
        polar.updateProjectionMatrix();
        expect(visibleTiles(polar, 'osm', 19, 64)).toEqual([]);
    });

    it('loads viewport NASA textures as meshes and holds them until OSM coverage is complete', async () => {
        const camera = cameraAt(48.8566, 2.3522, 1280 / 720);
        camera.zoom = 4;
        camera.updateProjectionMatrix();
        const nasaSource = detailSourceForCamera(camera, 1280, 2, 8192)!;
        expect(nasaSource.provider).toBe('nasa');
        const nasaTiles = visibleTiles(camera, 'nasa', nasaSource.level, 64);
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 1280 }, clientHeight: { value: 720 } });
        const scene = new THREE.Scene();
        const tracking = meshWaiter();
        const fetchMock = vi.fn<(url: RequestInfo | URL) => Promise<Response>>(async () => tileResponse());
        const bitmaps: ImageBitmap[] = [];
        vi.stubGlobal('fetch', fetchMock);
        vi.stubGlobal(
            'createImageBitmap',
            vi.fn(async () => {
                const bitmap = { close: vi.fn(), width: 512, height: 512 } as unknown as ImageBitmap;
                bitmaps.push(bitmap);
                return bitmap;
            }),
        );
        const sources = vi.fn();
        const fallbackMarked = deferred<void>();
        const fallbackStatus = vi.fn((unavailable: boolean) => {
            if (unavailable) fallbackMarked.resolve();
        });
        const layer = createGlobeDetailLayer(
            scene,
            camera,
            host,
            2,
            8192,
            4,
            false,
            tracking.render,
            fallbackStatus,
            sources,
        );
        tracking.setLayer(scene.children.find((child): child is THREE.Group => child instanceof THREE.Group)!);

        const nasaComplete = tracking.waitFor(() => tracking.meshes().length === nasaTiles.length);
        layer.scheduleUpdate();
        await vi.advanceTimersByTimeAsync(120);
        await nasaComplete;
        expect(fetchMock).toHaveBeenCalledTimes(nasaTiles.length);
        expect(fetchMock.mock.calls.every(([url]) => String(url).includes('gibs.earthdata.nasa.gov'))).toBe(true);
        expect(bitmaps).toHaveLength(nasaTiles.length);
        expect(sources).toHaveBeenLastCalledWith(['nasa']);

        camera.zoom = 32;
        camera.updateProjectionMatrix();
        const osmSource = detailSourceForCamera(camera, 1280, 2, 8192)!;
        expect(osmSource.provider).toBe('osm');
        const osmTiles = visibleTiles(camera, 'osm', osmSource.level, 64);
        expect(osmTiles.length).toBeGreaterThan(1);
        const heldOsm = deferred<Response>();
        let osmRequests = 0;
        fetchMock.mockImplementation(async (url) => {
            if (String(url).includes('gibs.earthdata.nasa.gov')) return tileResponse();
            osmRequests += 1;
            return osmRequests === 1 ? heldOsm.promise : tileResponse();
        });
        const countOsmMeshes = () => tracking.meshes((mesh) => meshRadius(mesh) > 1.003).length;
        const partialOsm = tracking.waitFor(() => countOsmMeshes() >= osmTiles.length - 1);
        camera.updateMatrixWorld(true);
        layer.scheduleUpdate();
        await vi.advanceTimersByTimeAsync(120);
        await partialOsm;
        expect(tracking.meshes((mesh) => meshRadius(mesh) <= 1.003).length).toBeGreaterThan(0);
        expect(tracking.meshes((mesh) => meshRadius(mesh) <= 1.003).length).toBeLessThanOrEqual(16);
        expect(sources).toHaveBeenLastCalledWith(['nasa', 'osm']);

        heldOsm.resolve(tileResponse());
        const completeOsm = tracking.waitFor(() => countOsmMeshes() === osmTiles.length);
        await completeOsm;
        expect(tracking.meshes((mesh) => meshRadius(mesh) <= 1.003)).toHaveLength(0);
        expect(sources).toHaveBeenLastCalledWith(['osm']);

        camera.zoom = 4;
        camera.updateProjectionMatrix();
        camera.updateMatrixWorld(true);
        fetchMock.mockImplementation(async () => ({ ok: false, status: 503 }) as Response);
        layer.scheduleUpdate();
        await vi.advanceTimersByTimeAsync(120);
        await fallbackMarked.promise;
        expect(tracking.meshes((mesh) => meshRadius(mesh) <= 1.003)).toHaveLength(0);
        expect(sources).toHaveBeenLastCalledWith(['osm']);
        expect(fallbackStatus).toHaveBeenCalledWith(true);
        layer.dispose();
        expect(bitmaps.every((bitmap) => vi.mocked(bitmap.close).mock.calls.length === 1)).toBe(true);
    });

    it('aborts stale requests, bounds mobile concurrency at 700px, and closes a stale bitmap', async () => {
        const camera = cameraAt(48, 2, 700 / 500);
        camera.zoom = 32;
        camera.updateProjectionMatrix();
        const host = document.createElement('div');
        Object.defineProperties(host, { clientWidth: { value: 700 }, clientHeight: { value: 500 } });
        const scene = new THREE.Scene();
        const tracking = meshWaiter();
        const requests: { signal: AbortSignal; pending: ReturnType<typeof deferred<Response>> }[] = [];
        const fetchMock = vi.fn((_url: RequestInfo | URL, init?: RequestInit) => {
            const pending = deferred<Response>();
            const signal = init?.signal as AbortSignal;
            requests.push({ signal, pending });
            signal.addEventListener('abort', () => pending.reject(new DOMException('Aborted', 'AbortError')), {
                once: true,
            });
            return pending.promise;
        });
        const bitmapStarted = deferred<void>();
        const staleBitmapReady = deferred<ImageBitmap>();
        const staleClosed = deferred<void>();
        const staleBitmap = {
            width: 256,
            height: 256,
            close: () => staleClosed.resolve(),
        } as ImageBitmap;
        let bitmapCalls = 0;
        vi.stubGlobal('fetch', fetchMock);
        vi.stubGlobal('createImageBitmap', () => {
            bitmapCalls += 1;
            bitmapStarted.resolve();
            return staleBitmapReady.promise;
        });
        const layer = createGlobeDetailLayer(
            scene,
            camera,
            host,
            2,
            2048,
            4,
            isGlobeMobile(700, true),
            tracking.render,
            vi.fn(),
            vi.fn(),
        );
        layer.scheduleUpdate();
        await vi.advanceTimersByTimeAsync(120);
        expect(fetchMock).toHaveBeenCalledTimes(4);
        expect(visibleTiles(camera, 'osm', 19, 32).length).toBeGreaterThanOrEqual(4);

        const firstRequest = requests[0];
        firstRequest.pending.resolve(tileResponse());
        await bitmapStarted.promise;
        camera.position.copy(globePosition(-45, 140, 3.5));
        camera.lookAt(0, 0, 0);
        camera.updateMatrixWorld(true);
        layer.scheduleUpdate();
        await vi.advanceTimersByTimeAsync(120);
        expect(firstRequest.signal.aborted).toBe(true);
        staleBitmapReady.resolve(staleBitmap);
        await staleClosed.promise;
        expect(bitmapCalls).toBe(1);
        expect(tracking.meshes()).toHaveLength(0);

        layer.dispose();
        expect(requests.every((request) => request.signal.aborted)).toBe(true);
        expect(scene.children).toHaveLength(0);
    });
});
