import * as THREE from 'three';
import { detailSourceForCamera, DETAIL_TILE_MAX_LEVEL, OSM_TILE_MAX_ZOOM, TILE_MATRICES } from './globeZoom';

const NASA_GIBS_TILE_BASE_URL =
    'https://gibs.earthdata.nasa.gov/wmts/epsg4326/best/BlueMarble_NextGeneration/default/500m';
const OSM_TILE_BASE_URL = 'https://tile.openstreetmap.org';
const GLOBE_ZOOM_UPDATE_DELAY_MS = 120;

type GlobeDetailProvider = 'nasa' | 'osm';

export interface GlobeDetailTile {
    provider: GlobeDetailProvider;
    level: number;
    row: number;
    column: number;
}

interface TileRecord {
    tile: GlobeDetailTile;
    controller: AbortController;
    mesh?: THREE.Mesh<THREE.BufferGeometry, THREE.MeshBasicMaterial>;
    bitmap?: ImageBitmap;
}

const TILE_SEGMENTS = 8;
const MAX_DESKTOP_TILES = 64;
const MAX_MOBILE_TILES = 32;
const MAX_DESKTOP_CONCURRENT_LOADS = 6;
const MAX_MOBILE_CONCURRENT_LOADS = 4;
const MAX_OSM_FALLBACK_TILES = 8;
const MAX_NASA_FALLBACK_TILES = 16;
const MERCATOR_MAX_LATITUDE = 85.05112878;

const radians = (degrees: number) => THREE.MathUtils.degToRad(degrees);
const degrees = (radiansValue: number) => THREE.MathUtils.radToDeg(radiansValue);

function tileKey(tile: GlobeDetailTile): string {
    return `${tile.provider}/${tile.level}/${tile.row}/${tile.column}`;
}

function tileURL(tile: GlobeDetailTile): string {
    if (tile.provider === 'osm') return `${OSM_TILE_BASE_URL}/${tile.level}/${tile.column}/${tile.row}.png`;
    return `${NASA_GIBS_TILE_BASE_URL}/${tile.level}/${tile.row}/${tile.column}.jpg`;
}

function frustumSurfaceRadius(camera: THREE.PerspectiveCamera, distance: number): number {
    const verticalHalfFov = radians(camera.getEffectiveFOV() / 2);
    const horizontalHalfFov = Math.atan(Math.tan(verticalHalfFov) * camera.aspect);
    const diagonalHalfFov = Math.atan(Math.hypot(Math.tan(verticalHalfFov), Math.tan(horizontalHalfFov)));
    return Math.asin(Math.min(1, distance * Math.sin(diagonalHalfFov)));
}

function globePosition(latitude: number, longitude: number, radius = 1): THREE.Vector3 {
    const lat = radians(latitude);
    const lon = radians(longitude);
    return new THREE.Vector3(
        radius * Math.cos(lat) * Math.cos(lon),
        radius * Math.sin(lat),
        -radius * Math.cos(lat) * Math.sin(lon),
    );
}

function nasaVisibleTiles(camera: THREE.PerspectiveCamera, level: number, maxTiles: number): GlobeDetailTile[] {
    const matrix = TILE_MATRICES[level];
    if (!matrix) throw new RangeError(`Unsupported NASA GIBS tile level: ${level}`);
    const center = camera.position.clone().normalize();
    const radius = frustumSurfaceRadius(camera, camera.position.length());
    const tileRadius = radians(Math.hypot(180 / matrix.height, 360 / matrix.width) / 2);
    const minimumDot = Math.cos(Math.min(Math.PI / 2, radius + tileRadius));
    const candidates: { tile: GlobeDetailTile; dot: number }[] = [];

    for (let row = 0; row < matrix.height; row += 1) {
        const latitude = 90 - ((row + 0.5) * 180) / matrix.height;
        for (let column = 0; column < matrix.width; column += 1) {
            const longitude = -180 + ((column + 0.5) * 360) / matrix.width;
            const dot = center.dot(globePosition(latitude, longitude));
            if (dot <= 0 || dot < minimumDot) continue;
            candidates.push({ tile: { provider: 'nasa', level, row, column }, dot });
        }
    }

    return candidates
        .sort((left, right) => right.dot - left.dot)
        .slice(0, maxTiles)
        .map(({ tile }) => tile);
}

function longitudeForPoint(point: THREE.Vector3): number {
    return degrees(Math.atan2(-point.z, point.x));
}

function latitudeForPoint(point: THREE.Vector3): number {
    return degrees(Math.asin(THREE.MathUtils.clamp(point.y, -1, 1)));
}

function unwrapLongitude(longitude: number, around: number): number {
    return around + ((((longitude - around + 540) % 360) + 360) % 360) - 180;
}

function osmColumn(longitude: number, zoom: number): number {
    return Math.floor(((longitude + 180) / 360) * 2 ** zoom);
}

function osmRow(latitude: number, zoom: number): number {
    const lat = radians(THREE.MathUtils.clamp(latitude, -MERCATOR_MAX_LATITUDE, MERCATOR_MAX_LATITUDE));
    return Math.floor(((1 - Math.asinh(Math.tan(lat)) / Math.PI) / 2) * 2 ** zoom);
}

function osmTileCenter(column: number, row: number, zoom: number): { latitude: number; longitude: number } {
    const width = 2 ** zoom;
    const mercator = Math.PI * (1 - (2 * (row + 0.5)) / width);
    return {
        latitude: degrees(Math.atan(Math.sinh(mercator))),
        longitude: ((column + 0.5) / width) * 360 - 180,
    };
}

function osmVisibleTiles(camera: THREE.PerspectiveCamera, zoom: number, maxTiles: number): GlobeDetailTile[] {
    const center = camera.position.clone().normalize();
    const centerLongitude = longitudeForPoint(center);
    const points: THREE.Vector3[] = [];
    const raycaster = new THREE.Raycaster();
    const sphere = new THREE.Sphere(new THREE.Vector3(), 1);
    const samples = [
        [-1, -1],
        [0, -1],
        [1, -1],
        [1, 0],
        [1, 1],
        [0, 1],
        [-1, 1],
        [-1, 0],
        [0, 0],
    ];
    camera.updateMatrixWorld();
    for (const [x, y] of samples) {
        raycaster.setFromCamera(new THREE.Vector2(x, y), camera);
        const point = raycaster.ray.intersectSphere(sphere, new THREE.Vector3());
        // Keep NASA's imagery at the limb until every edge of the view lands on the surface.
        if (!point) return [];
        points.push(point.normalize());
    }

    const latitudes = points.map(latitudeForPoint);
    const longitudes = points.map((point) => unwrapLongitude(longitudeForPoint(point), centerLongitude));
    const pad = 1;
    const count = 2 ** zoom;
    let minColumn = osmColumn(Math.min(...longitudes), zoom) - pad;
    let maxColumn = osmColumn(Math.max(...longitudes), zoom) + pad;
    const minRow = THREE.MathUtils.clamp(osmRow(Math.max(...latitudes), zoom) - pad, 0, count - 1);
    const maxRow = THREE.MathUtils.clamp(osmRow(Math.min(...latitudes), zoom) + pad, 0, count - 1);
    if (maxColumn - minColumn + 1 > maxTiles * 4) {
        const middle = osmColumn(centerLongitude, zoom);
        minColumn = middle - Math.ceil(Math.sqrt(maxTiles));
        maxColumn = middle + Math.ceil(Math.sqrt(maxTiles));
    }

    const candidates: { tile: GlobeDetailTile; dot: number }[] = [];
    const radius = frustumSurfaceRadius(camera, camera.position.length());
    for (let rawColumn = minColumn; rawColumn <= maxColumn; rawColumn += 1) {
        const column = ((rawColumn % count) + count) % count;
        for (let row = minRow; row <= maxRow; row += 1) {
            const { latitude, longitude } = osmTileCenter(column, row, zoom);
            const dot = center.dot(globePosition(latitude, longitude));
            const tileWidth = (360 / count) * Math.cos(radians(latitude));
            const tileHeight = Math.abs(
                degrees(
                    Math.atan(Math.sinh(Math.PI * (1 - (2 * row) / count))) -
                        Math.atan(Math.sinh(Math.PI * (1 - (2 * (row + 1)) / count))),
                ),
            );
            const tileRadius = radians(Math.hypot(tileWidth, tileHeight) / 2);
            if (dot <= 0 || dot < Math.cos(Math.min(Math.PI / 2, radius + tileRadius))) continue;
            candidates.push({ tile: { provider: 'osm', level: zoom, row, column }, dot });
        }
    }

    const unique = new Map(candidates.map(({ tile, dot }) => [tileKey(tile), { tile, dot }]));
    return [...unique.values()]
        .sort((left, right) => right.dot - left.dot)
        .slice(0, maxTiles)
        .map(({ tile }) => tile);
}

export function visibleTiles(
    camera: THREE.PerspectiveCamera,
    provider: GlobeDetailProvider,
    level: number,
    maxTiles: number,
): GlobeDetailTile[] {
    if (provider === 'osm') {
        if (level < 0 || level > OSM_TILE_MAX_ZOOM) throw new RangeError(`Unsupported OSM tile zoom: ${level}`);
        return osmVisibleTiles(camera, level, maxTiles);
    }
    return nasaVisibleTiles(camera, level, maxTiles);
}

function tileBounds(tile: GlobeDetailTile) {
    if (tile.provider === 'osm') {
        const count = 2 ** tile.level;
        const latitudeNorth = degrees(Math.atan(Math.sinh(Math.PI * (1 - (2 * tile.row) / count))));
        const latitudeSouth = degrees(Math.atan(Math.sinh(Math.PI * (1 - (2 * (tile.row + 1)) / count))));
        return {
            latitudeNorth,
            latitudeSouth,
            longitudeWest: (tile.column / count) * 360 - 180,
            longitudeEast: ((tile.column + 1) / count) * 360 - 180,
        };
    }
    const matrix = TILE_MATRICES[tile.level];
    return {
        latitudeNorth: 90 - (tile.row * 180) / matrix.height,
        latitudeSouth: 90 - ((tile.row + 1) * 180) / matrix.height,
        longitudeWest: -180 + (tile.column * 360) / matrix.width,
        longitudeEast: -180 + ((tile.column + 1) * 360) / matrix.width,
    };
}

export function tileLatitudeAtRow(tile: GlobeDetailTile, rowProgress: number): number {
    const progress = THREE.MathUtils.clamp(rowProgress, 0, 1);
    if (tile.provider === 'nasa') {
        const bounds = tileBounds(tile);
        return bounds.latitudeNorth + (bounds.latitudeSouth - bounds.latitudeNorth) * progress;
    }
    const count = 2 ** tile.level;
    const mercatorY = Math.PI * (1 - (2 * (tile.row + progress)) / count);
    return degrees(Math.atan(Math.sinh(mercatorY)));
}

function tileRadius(tile: GlobeDetailTile): number {
    if (tile.provider === 'osm') return 1.003 + tile.level * 0.00002;
    return 1.002 + (tile.level / DETAIL_TILE_MAX_LEVEL) * 0.0005;
}

function tileGeometry(tile: GlobeDetailTile): THREE.BufferGeometry {
    const bounds = tileBounds(tile);
    const side = TILE_SEGMENTS + 1;
    const vertices = new Float32Array(side * side * 3);
    const uvs = new Float32Array(side * side * 2);
    let vertex = 0;
    let uv = 0;

    for (let row = 0; row <= TILE_SEGMENTS; row += 1) {
        const latitude = tileLatitudeAtRow(tile, row / TILE_SEGMENTS);
        for (let column = 0; column <= TILE_SEGMENTS; column += 1) {
            const longitude =
                bounds.longitudeWest + ((bounds.longitudeEast - bounds.longitudeWest) * column) / TILE_SEGMENTS;
            const position = globePosition(latitude, longitude, tileRadius(tile));
            vertices[vertex++] = position.x;
            vertices[vertex++] = position.y;
            vertices[vertex++] = position.z;
            uvs[uv++] = column / TILE_SEGMENTS;
            uvs[uv++] = 1 - row / TILE_SEGMENTS;
        }
    }

    const indices: number[] = [];
    for (let row = 0; row < TILE_SEGMENTS; row += 1) {
        for (let column = 0; column < TILE_SEGMENTS; column += 1) {
            const topLeft = row * side + column;
            const topRight = topLeft + 1;
            const bottomLeft = topLeft + side;
            const bottomRight = bottomLeft + 1;
            indices.push(topLeft, bottomLeft, topRight, topRight, bottomLeft, bottomRight);
        }
    }

    const geometry = new THREE.BufferGeometry();
    geometry.setAttribute('position', new THREE.BufferAttribute(vertices, 3));
    geometry.setAttribute('uv', new THREE.BufferAttribute(uvs, 2));
    geometry.setIndex(indices);
    geometry.computeVertexNormals();
    return geometry;
}

export function createGlobeDetailLayer(
    scene: THREE.Scene,
    camera: THREE.PerspectiveCamera,
    host: HTMLDivElement,
    pixelRatio: number,
    baseTextureSize: number,
    maxAnisotropy: number,
    mobile: boolean,
    render: () => void,
    onFallback: (unavailable: boolean) => void,
    onSources: (providers: GlobeDetailProvider[]) => void,
) {
    let disposed = false;
    let scheduleID: number | undefined;
    let loadCount = 0;
    let fallbackReported = false;
    let reportedSources = '';
    const failedTiles = new Set<string>();
    let wantedKeys = new Set<string>();
    const tiles = new Map<string, TileRecord>();
    const pending: TileRecord[] = [];
    const layer = new THREE.Group();
    const maxTiles = mobile ? MAX_MOBILE_TILES : MAX_DESKTOP_TILES;
    const maxConcurrentLoads = mobile ? MAX_MOBILE_CONCURRENT_LOADS : MAX_DESKTOP_CONCURRENT_LOADS;
    scene.add(layer);

    const setFallback = (unavailable: boolean) => {
        if (fallbackReported === unavailable) return;
        fallbackReported = unavailable;
        onFallback(unavailable);
    };

    const reportVisibleSources = () => {
        const providers = [
            ...new Set([...tiles.values()].filter((record) => record.mesh).map((record) => record.tile.provider)),
        ].sort();
        const key = providers.join(',');
        if (reportedSources === key) return;
        reportedSources = key;
        onSources(providers);
    };

    const retireFallbackMeshes = () => {
        if (wantedKeys.size === 0 || ![...wantedKeys].every((key) => tiles.get(key)?.mesh)) return;
        for (const [key, record] of tiles) {
            if (wantedKeys.has(key)) continue;
            disposeRecord(record);
            tiles.delete(key);
        }
    };

    const disposeRecord = (record: TileRecord) => {
        record.controller.abort();
        if (record.mesh) {
            layer.remove(record.mesh);
            record.mesh.geometry.dispose();
            record.mesh.material.map?.dispose();
            record.mesh.material.dispose();
            record.mesh = undefined;
        }
        record.bitmap?.close();
        record.bitmap = undefined;
    };

    const loadTile = async (record: TileRecord) => {
        const response = await fetch(tileURL(record.tile), {
            signal: record.controller.signal,
            mode: 'cors',
            credentials: 'omit',
            cache: 'default',
        });
        if (!response.ok) throw new Error(`Globe ${record.tile.provider} tile request failed: ${response.status}`);
        const blob = await response.blob();
        if (record.controller.signal.aborted || disposed) return;
        if (typeof createImageBitmap !== 'function') throw new Error('ImageBitmap is unavailable');
        const bitmap = await createImageBitmap(blob, { imageOrientation: 'flipY' });
        if (record.controller.signal.aborted || disposed || tiles.get(tileKey(record.tile)) !== record) {
            bitmap.close();
            return;
        }
        record.bitmap = bitmap;
        const texture = new THREE.Texture(bitmap);
        texture.flipY = false;
        texture.colorSpace = THREE.SRGBColorSpace;
        texture.minFilter = THREE.LinearMipmapLinearFilter;
        texture.magFilter = THREE.LinearFilter;
        texture.anisotropy = Math.min(maxAnisotropy, 2);
        texture.needsUpdate = true;
        const material = new THREE.MeshBasicMaterial({ map: texture });
        const mesh = new THREE.Mesh(tileGeometry(record.tile), material);
        mesh.frustumCulled = false;
        record.mesh = mesh;
        layer.add(mesh);
        retireFallbackMeshes();
        reportVisibleSources();
        failedTiles.delete(tileKey(record.tile));
        if (failedTiles.size === 0) setFallback(false);
        render();
    };

    const drainQueue = () => {
        while (!disposed && loadCount < maxConcurrentLoads && pending.length > 0) {
            const record = pending.shift();
            if (!record || record.controller.signal.aborted || tiles.get(tileKey(record.tile)) !== record) continue;
            loadCount += 1;
            void loadTile(record)
                .catch((error: unknown) => {
                    const wasAborted = error instanceof Error && error.name === 'AbortError';
                    if (wasAborted || record.controller.signal.aborted || disposed) return;
                    tiles.delete(tileKey(record.tile));
                    failedTiles.add(tileKey(record.tile));
                    setFallback(true);
                })
                .finally(() => {
                    loadCount -= 1;
                    drainQueue();
                });
        }
    };

    const update = () => {
        if (disposed || host.clientWidth <= 0 || host.clientHeight <= 0) return;
        const source = detailSourceForCamera(camera, host.clientWidth, pixelRatio, baseTextureSize);
        if (!source) {
            for (const record of tiles.values()) disposeRecord(record);
            tiles.clear();
            pending.length = 0;
            wantedKeys = new Set();
            reportVisibleSources();
            failedTiles.clear();
            setFallback(false);
            render();
            return;
        }
        const wanted = visibleTiles(camera, source.provider, source.level, maxTiles);
        wantedKeys = new Set(wanted.map(tileKey));
        for (const [key, record] of tiles) {
            const canFallback = Boolean(record.mesh) && !wantedKeys.has(key);
            if (!wantedKeys.has(key) && !canFallback) {
                disposeRecord(record);
                tiles.delete(key);
            }
        }
        for (const key of failedTiles) {
            if (!wantedKeys.has(key)) failedTiles.delete(key);
        }
        if (failedTiles.size === 0) setFallback(false);

        // Keep a small number of lower-resolution visible tiles while sharper
        // tiles arrive. They sit below the new source and the bundled texture
        // remains visible beneath both, so a slow or offline tile never blanks Earth.
        const fallbackLimit = source.provider === 'osm' ? MAX_NASA_FALLBACK_TILES : MAX_OSM_FALLBACK_TILES;
        const fallback = [...tiles.entries()].filter(([key]) => !wantedKeys.has(key));
        while (fallback.length > fallbackLimit) {
            const [key, record] = fallback.shift()!;
            disposeRecord(record);
            tiles.delete(key);
        }

        pending.splice(
            0,
            pending.length,
            ...pending.filter(
                (record) =>
                    wantedKeys.has(tileKey(record.tile)) &&
                    !record.controller.signal.aborted &&
                    tiles.get(tileKey(record.tile)) === record,
            ),
        );
        for (const tile of wanted) {
            const key = tileKey(tile);
            if (tiles.has(key)) continue;
            const record = { tile, controller: new AbortController() };
            tiles.set(key, record);
            pending.push(record);
        }
        retireFallbackMeshes();
        reportVisibleSources();
        for (const [key, record] of tiles) {
            if (!wantedKeys.has(key) && pending.includes(record)) {
                pending.splice(pending.indexOf(record), 1);
                record.controller.abort();
                tiles.delete(key);
            }
        }
        drainQueue();
    };

    return {
        scheduleUpdate() {
            if (disposed) return;
            if (scheduleID !== undefined) window.clearTimeout(scheduleID);
            scheduleID = window.setTimeout(() => {
                scheduleID = undefined;
                update();
            }, GLOBE_ZOOM_UPDATE_DELAY_MS);
        },
        dispose() {
            if (disposed) return;
            disposed = true;
            if (scheduleID !== undefined) window.clearTimeout(scheduleID);
            for (const record of tiles.values()) disposeRecord(record);
            tiles.clear();
            wantedKeys.clear();
            failedTiles.clear();
            pending.length = 0;
            reportVisibleSources();
            scene.remove(layer);
        },
    };
}
