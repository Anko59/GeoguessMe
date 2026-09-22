import * as THREE from 'three';

const DETAIL_TILE_SIZE = 512;
export const DETAIL_TILE_MAX_LEVEL = 7;
const DETAIL_TILE_MAX_PIXELS_PER_DEGREE = (160 * DETAIL_TILE_SIZE) / 360;
const OSM_TILE_SIZE = 256;
export const OSM_TILE_MAX_ZOOM = 19;
export const OSM_TILE_MAX_PIXELS_PER_DEGREE = (OSM_TILE_SIZE * 2 ** OSM_TILE_MAX_ZOOM) / 360;

interface TileMatrix {
    width: number;
    height: number;
}

export const TILE_MATRICES: TileMatrix[] = [
    { width: 2, height: 1 },
    { width: 3, height: 2 },
    { width: 5, height: 3 },
    { width: 10, height: 5 },
    { width: 20, height: 10 },
    { width: 40, height: 20 },
    { width: 80, height: 40 },
    { width: 160, height: 80 },
];

export function isGlobeMobile(viewportWidth: number, coarsePointer: boolean): boolean {
    return viewportWidth <= 768 || coarsePointer;
}

const radians = (degrees: number) => THREE.MathUtils.degToRad(degrees);
const degrees = (radiansValue: number) => THREE.MathUtils.radToDeg(radiansValue);

export function tilePixelsPerDegree(level: number): number {
    const matrix = TILE_MATRICES[level];
    if (!matrix) throw new RangeError(`Unsupported NASA GIBS tile level: ${level}`);
    return (matrix.width * DETAIL_TILE_SIZE) / 360;
}

export function visibleSurfaceWidthDegrees(camera: THREE.PerspectiveCamera, distance = camera.position.length()) {
    const halfFov = radians(camera.getEffectiveFOV() / 2);
    const halfHorizontalFov = Math.atan(Math.tan(halfFov) * camera.aspect);
    const halfWidthOnGlobe = Math.asin(Math.min(1, distance * Math.sin(halfHorizontalFov)));
    return degrees(halfWidthOnGlobe * 2);
}

export function detailSourceForCamera(
    camera: THREE.PerspectiveCamera,
    viewportWidth: number,
    pixelRatio: number,
    baseTextureSize: number,
): { provider: 'nasa' | 'osm'; level: number } | null {
    const widthDegrees = visibleSurfaceWidthDegrees(camera);
    const displayPixelsPerDegree = (viewportWidth * pixelRatio) / Math.max(widthDegrees, 1e-6);
    const neededPixelsPerDegree = displayPixelsPerDegree * 1.2;
    if (neededPixelsPerDegree <= baseTextureSize / 360) return null;
    if (neededPixelsPerDegree <= DETAIL_TILE_MAX_PIXELS_PER_DEGREE) {
        const level = TILE_MATRICES.findIndex(
            (matrix) => (matrix.width * DETAIL_TILE_SIZE) / 360 >= neededPixelsPerDegree,
        );
        return { provider: 'nasa', level: level < 0 ? DETAIL_TILE_MAX_LEVEL : level };
    }
    const level = Math.ceil(Math.log2((neededPixelsPerDegree * 360) / OSM_TILE_SIZE));
    return { provider: 'osm', level: THREE.MathUtils.clamp(level, 0, OSM_TILE_MAX_ZOOM) };
}

export function maxUsefulGlobeZoom(
    camera: THREE.PerspectiveCamera,
    viewportWidth: number,
    pixelRatio: number,
    distance = camera.position.length(),
): number {
    const desiredWidthDegrees = (viewportWidth * pixelRatio * 1.2) / OSM_TILE_MAX_PIXELS_PER_DEGREE;
    const sinHalfSurfaceWidth = Math.sin(radians(desiredWidthDegrees / 2));
    const halfHorizontalFov = Math.asin(Math.min(1, sinHalfSurfaceWidth / distance));
    const baseHorizontalTangent = Math.tan(radians(camera.fov / 2)) * camera.aspect;
    return THREE.MathUtils.clamp(baseHorizontalTangent / Math.tan(halfHorizontalFov), 1, 1_000_000);
}
