import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import type { GroupChallenge } from '../../types';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../utils/mapPins';
import { createGlobeDetailLayer } from './globeDetailTiles';
import { isGlobeMobile, maxUsefulGlobeZoom } from './globeZoom';

export const HIGH_RES_EARTH_TEXTURE_URL = '/globe/earth-8192.jpg';
export const FALLBACK_EARTH_TEXTURE_URL = '/globe/earth.jpg';
export const HIGH_RES_TEXTURE_SIZE = 8192;
export const HIGH_RES_MIN_DISTANCE = 1.05;
export const FALLBACK_MIN_DISTANCE = 1.15;

export function earthTextureURL(maxTextureSize: number, mobile = false): string {
    return maxTextureSize >= HIGH_RES_TEXTURE_SIZE && !mobile ? HIGH_RES_EARTH_TEXTURE_URL : FALLBACK_EARTH_TEXTURE_URL;
}

export function earthMinDistance(maxTextureSize: number, mobile = false): number {
    return maxTextureSize >= HIGH_RES_TEXTURE_SIZE && !mobile ? HIGH_RES_MIN_DISTANCE : FALLBACK_MIN_DISTANCE;
}

export function earthTextureSize(maxTextureSize: number, mobile = false): number {
    return maxTextureSize >= HIGH_RES_TEXTURE_SIZE && !mobile ? HIGH_RES_TEXTURE_SIZE : 2048;
}

// Matches the equirectangular texture's Greenwich meridian and north pole.
export function globePosition(lat: number, long: number, radius = 1): THREE.Vector3 {
    const latitude = THREE.MathUtils.degToRad(lat);
    const longitude = THREE.MathUtils.degToRad(long);
    return new THREE.Vector3(
        radius * Math.cos(latitude) * Math.cos(longitude),
        radius * Math.sin(latitude),
        -radius * Math.cos(latitude) * Math.sin(longitude),
    );
}

// Keeps the camera a few degrees away from the degenerate poles, where a
// horizontal drag would spin the globe in place instead of moving the view.
const POLE_MARGIN = 0.05;

export function createGlobeScene(
    host: HTMLDivElement,
    onSelect: (id: string) => void,
    onError: (message: string) => void,
    onReady?: () => void,
    onDetailFallback?: (unavailable: boolean) => void,
    onDetailSources?: (providers: ('nasa' | 'osm')[]) => void,
) {
    let disposed = false;
    const cleanup: (() => void)[] = [];
    const dispose = () => {
        if (disposed) return;
        disposed = true;
        for (const release of cleanup.reverse()) release();
    };
    try {
        const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
        cleanup.push(() => {
            renderer.dispose();
            renderer.forceContextLoss();
            renderer.domElement.remove();
        });
        const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
        renderer.setPixelRatio(pixelRatio);
        renderer.domElement.setAttribute('aria-hidden', 'true');
        host.appendChild(renderer.domElement);
        const pinOverlay = document.createElement('canvas');
        pinOverlay.className = 'globe-pin-overlay';
        pinOverlay.setAttribute('aria-hidden', 'true');
        const pinOverlayContext = pinOverlay.getContext('2d');
        if (!pinOverlayContext) throw new Error('Canvas 2D is unavailable');
        host.appendChild(pinOverlay);
        cleanup.push(() => {
            pinOverlay.width = 0;
            pinOverlay.height = 0;
            pinOverlay.remove();
        });
        const scene = new THREE.Scene();
        const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 50);
        camera.position.copy(globePosition(22, 12, 3.5));
        camera.zoom = 1;
        camera.updateProjectionMatrix();
        const controls = new OrbitControls(camera, renderer.domElement);
        cleanup.push(() => controls.dispose());
        controls.enablePan = false;
        // Zoom is controlled through camera.zoom so the view can move beyond
        // the old camera-distance limit without entering the Earth mesh.
        controls.enableZoom = false;
        const maxTextureSize = renderer.capabilities.maxTextureSize;
        const mobile = isGlobeMobile(host.clientWidth, window.matchMedia('(pointer: coarse)').matches);
        const baseTextureSize = earthTextureSize(maxTextureSize, mobile);
        controls.minDistance = earthMinDistance(maxTextureSize, mobile);
        controls.maxDistance = 6;
        controls.touches = { ONE: THREE.TOUCH.ROTATE, TWO: THREE.TOUCH.DOLLY_PAN };
        controls.zoomToCursor = false;
        controls.minPolarAngle = POLE_MARGIN;
        controls.maxPolarAngle = Math.PI - POLE_MARGIN;
        // Render only on interaction: no idle animation or motion preference override.
        controls.enableDamping = false;
        // OrbitControls' linear pixel mapping is tuned for flat planes: at its
        // default speed a short finger drag overshoots several time zones. Scale
        // the rotation speed so dragging moves the surface point under the
        // finger one to one: rotateSpeed = tan(fov/2)·√(d²−1)/π for radius 1.
        const syncControls = () => {
            const distance = camera.position.length();
            const effectiveFov = 2 * Math.atan(Math.tan(THREE.MathUtils.degToRad(camera.fov / 2)) / camera.zoom);
            controls.rotateSpeed = Math.min(
                (Math.tan(effectiveFov / 2) * Math.sqrt(Math.max(distance * distance - 1, 0.0025))) / Math.PI,
                1,
            );
        };
        syncControls();
        // Damping gives touch gestures their glide, but only while a gesture is
        // in flight: the loop below stops as soon as motion settles, preserving
        // the interaction-driven rendering contract and reduced-motion choices.
        const motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)');
        let frame = 0;
        const step = () => {
            frame = 0;
            if (disposed) return;
            const before = camera.position.clone();
            controls.update();
            syncControls();
            if (camera.position.distanceToSquared(before) < 1e-8) {
                controls.enableDamping = false;
                return;
            }
            frame = requestAnimationFrame(step);
        };
        const beginInteraction = () => {
            if (motionQuery.matches) return;
            controls.enableDamping = true;
            syncControls();
            if (!frame) frame = requestAnimationFrame(step);
        };
        controls.addEventListener('start', beginInteraction);
        cleanup.push(() => {
            controls.removeEventListener('start', beginInteraction);
            if (frame) cancelAnimationFrame(frame);
        });
        const render = () => renderer.render(scene, camera);
        let scheduleDetailUpdate = () => {};
        let syncPinOverlay = () => {};
        const syncDetailTiles = () => {
            syncPinOverlay();
            scheduleDetailUpdate();
        };
        controls.addEventListener('change', render);
        controls.addEventListener('change', syncDetailTiles);
        cleanup.push(() => {
            controls.removeEventListener('change', render);
            controls.removeEventListener('change', syncDetailTiles);
        });
        const earthGeometry = new THREE.SphereGeometry(1, 96, 64);
        cleanup.push(() => earthGeometry.dispose());
        const earthMaterial = new THREE.MeshPhongMaterial({ color: 0xb9d9f5, shininess: 8 });
        cleanup.push(() => earthMaterial.dispose());
        const earth = new THREE.Mesh(earthGeometry, earthMaterial);
        scene.add(earth);
        scene.add(new THREE.AmbientLight(0xffffff, 2));
        const light = new THREE.DirectionalLight(0xffffff, 2);
        light.position.set(-3, 5, 4);
        scene.add(light);
        const texture = new THREE.TextureLoader().load(
            earthTextureURL(maxTextureSize, mobile),
            (loaded) => {
                if (disposed) return;
                loaded.colorSpace = THREE.SRGBColorSpace;
                loaded.minFilter = THREE.LinearMipmapLinearFilter;
                loaded.magFilter = THREE.LinearFilter;
                loaded.anisotropy = Math.min(renderer.capabilities.getMaxAnisotropy(), 4);
                earthMaterial.map = loaded;
                earthMaterial.color.set(0xffffff);
                earthMaterial.needsUpdate = true;
                render();
                // The globe is only interactive once the Earth texture has
                // decoded and rendered; surface that here instead of letting
                // callers infer it from scene construction.
                onReady?.();
            },
            undefined,
            () => {
                if (!disposed) onError('The Earth texture could not load. Your challenge list is still available.');
            },
        );
        cleanup.push(() => texture.dispose());
        let visible: GroupChallenge[] = [];
        let positions: THREE.Vector3[] = [];
        let pinArtwork: HTMLImageElement[] = [];
        let previousItems: GroupChallenge[] | undefined;
        let selectedIndex: number | undefined;
        const pinIndexes = new Map<string, number>();
        // One decoded image per distinct equipped-pin asset, released with this scene.
        const artworkByURL = new Map<string, HTMLImageElement>();
        const disposePinArtwork = () => {
            for (const image of artworkByURL.values()) {
                image.onload = null;
                image.onerror = null;
                image.removeAttribute('src');
            }
            artworkByURL.clear();
            pinArtwork = [];
        };
        cleanup.push(disposePinArtwork);
        const cameraPosition = new THREE.Vector3();
        const pinNormal = new THREE.Vector3();
        const projectedMarker = new THREE.Vector3();
        let pinScreenPositions = new Float32Array();
        let pinScreenVisible = new Uint8Array();
        syncPinOverlay = () => {
            const width = host.clientWidth;
            const height = host.clientHeight;
            pinOverlayContext.clearRect(0, 0, width, height);
            pinScreenVisible.fill(0);
            if (width <= 0 || height <= 0) return;
            camera.updateMatrixWorld();
            const tanHalfFov = Math.tan(THREE.MathUtils.degToRad(camera.getEffectiveFOV() / 2));
            for (let index = 0; index < positions.length; index += 1) {
                cameraPosition.copy(positions[index]).applyMatrix4(camera.matrixWorldInverse);
                const depth = Math.max(-cameraPosition.z, 0.1);
                const diameter = index === selectedIndex ? 8 : 6;
                const radius = (diameter * depth * tanHalfFov) / height;
                const markerHeight = radius * (index === selectedIndex ? 8 : 7);
                pinNormal.copy(positions[index]).normalize();
                projectedMarker
                    .copy(positions[index])
                    .addScaledVector(pinNormal, markerHeight * 0.48)
                    .project(camera);
                const frontFacing = positions[index].dot(camera.position) > positions[index].lengthSq();
                const inView =
                    Math.abs(projectedMarker.x) <= 1 &&
                    Math.abs(projectedMarker.y) <= 1 &&
                    projectedMarker.z >= -1 &&
                    projectedMarker.z <= 1;
                if (!frontFacing || !inView) continue;

                const x = ((projectedMarker.x + 1) * width) / 2;
                const y = ((1 - projectedMarker.y) * height) / 2;
                pinScreenPositions[index * 2] = x;
                pinScreenPositions[index * 2 + 1] = y;
                pinScreenVisible[index] = 1;

                const image = pinArtwork[index];
                if (!image?.complete || image.naturalWidth <= 0) continue;
                const imageHeight = (markerHeight * height) / (2 * depth * tanHalfFov);
                const imageWidth = imageHeight * 0.78;
                pinOverlayContext.drawImage(image, x - imageWidth / 2, y - imageHeight / 2, imageWidth, imageHeight);
            }
        };
        const getPinArtwork = (url: string) => {
            const cached = artworkByURL.get(url);
            if (cached) return cached;
            const image = new Image();
            image.decoding = 'async';
            image.onload = () => {
                if (!disposed) syncPinOverlay();
            };
            image.onerror = () => {
                if (image.getAttribute('src') !== DEFAULT_MAP_PIN_IMAGE_URL) {
                    image.src = DEFAULT_MAP_PIN_IMAGE_URL;
                    return;
                }
                if (!disposed) onError('Challenge pin artwork could not load. Your challenge list is still available.');
            };
            artworkByURL.set(url, image);
            image.src = url;
            return image;
        };
        const detailLayer = createGlobeDetailLayer(
            scene,
            camera,
            host,
            pixelRatio,
            baseTextureSize,
            renderer.capabilities.getMaxAnisotropy(),
            mobile,
            render,
            (unavailable) => onDetailFallback?.(unavailable),
            (providers) => onDetailSources?.(providers),
        );
        scheduleDetailUpdate = () => detailLayer.scheduleUpdate();
        cleanup.push(() => detailLayer.dispose());
        const resize = () => {
            const width = Math.max(host.clientWidth, 1);
            const height = Math.max(host.clientHeight, 1);
            camera.aspect = width / height;
            camera.updateProjectionMatrix();
            renderer.setSize(width, height);
            pinOverlay.width = Math.round(width * pixelRatio);
            pinOverlay.height = Math.round(height * pixelRatio);
            pinOverlayContext.setTransform(pixelRatio, 0, 0, pixelRatio, 0, 0);
            syncControls();
            syncPinOverlay();
            render();
            detailLayer.scheduleUpdate();
        };
        const observer = new ResizeObserver(resize);
        cleanup.push(() => observer.disconnect());
        observer.observe(host);
        let pointerStart: { x: number; y: number } | null = null;
        let dragged = false;
        const touchPointers = new Map<number, { x: number; y: number }>();
        let previousPinchDistance: number | null = null;
        const applyZoom = (factor: number) => {
            camera.zoom = THREE.MathUtils.clamp(
                camera.zoom * factor,
                1,
                maxUsefulGlobeZoom(camera, host.clientWidth, pixelRatio),
            );
            camera.updateProjectionMatrix();
            syncControls();
            syncPinOverlay();
            render();
            detailLayer.scheduleUpdate();
        };
        const pointerDown = (event: PointerEvent) => {
            if (event.pointerType === 'touch') {
                touchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
                if (touchPointers.size === 2) {
                    const [first, second] = [...touchPointers.values()];
                    previousPinchDistance = Math.hypot(first.x - second.x, first.y - second.y);
                }
            }
            if (event.button !== 0) return;
            if (pointerStart) {
                dragged = true;
                return;
            }
            pointerStart = { x: event.clientX, y: event.clientY };
            dragged = false;
        };
        const pointerCancel = (event: PointerEvent) => {
            if (event.pointerType === 'touch') {
                touchPointers.delete(event.pointerId);
                previousPinchDistance = null;
            }
            pointerStart = null;
        };
        const pointerMove = (event: PointerEvent) => {
            if (event.pointerType === 'touch' && touchPointers.has(event.pointerId)) {
                touchPointers.set(event.pointerId, { x: event.clientX, y: event.clientY });
                if (touchPointers.size >= 2) {
                    const [first, second] = [...touchPointers.values()];
                    const distance = Math.hypot(first.x - second.x, first.y - second.y);
                    if (previousPinchDistance && distance > 0) applyZoom(distance / previousPinchDistance);
                    previousPinchDistance = distance;
                }
            }
            if (pointerStart && Math.hypot(event.clientX - pointerStart.x, event.clientY - pointerStart.y) > 6)
                dragged = true;
        };
        const pointerUp = (event: PointerEvent) => {
            if (event.pointerType === 'touch') {
                touchPointers.delete(event.pointerId);
                previousPinchDistance = null;
            }
            if (!pointerStart || dragged) {
                pointerStart = null;
                return;
            }
            pointerStart = null;
            const bounds = renderer.domElement.getBoundingClientRect();
            // Reuse the canvas overlay's front-facing projection for 44px hit
            // targets. This avoids per-pin GPU hit meshes and excludes pins
            // hidden by the globe without another projection pass.
            let nearest = 22;
            let candidate: number | undefined;
            for (let index = 0; index < visible.length; index += 1) {
                if (!pinScreenVisible[index]) continue;
                const x = bounds.left + pinScreenPositions[index * 2];
                const y = bounds.top + pinScreenPositions[index * 2 + 1];
                const distance = Math.hypot(event.clientX - x, event.clientY - y);
                if (distance < nearest) {
                    nearest = distance;
                    candidate = index;
                }
            }
            if (candidate !== undefined) onSelect(visible[candidate].photo_id);
        };
        const contextLost = (event: Event) => {
            event.preventDefault();
            onError('3D rendering is unavailable. You can still browse every challenge below.');
        };
        const wheelZoom = (event: WheelEvent) => {
            event.preventDefault();
            applyZoom(Math.exp(-event.deltaY * 0.002));
        };
        renderer.domElement.addEventListener('pointerdown', pointerDown);
        renderer.domElement.addEventListener('pointermove', pointerMove);
        renderer.domElement.addEventListener('pointerup', pointerUp);
        renderer.domElement.addEventListener('pointercancel', pointerCancel);
        renderer.domElement.addEventListener('webglcontextlost', contextLost);
        renderer.domElement.addEventListener('wheel', wheelZoom, { passive: false });
        cleanup.push(() => {
            renderer.domElement.removeEventListener('pointerdown', pointerDown);
            renderer.domElement.removeEventListener('pointermove', pointerMove);
            renderer.domElement.removeEventListener('pointerup', pointerUp);
            renderer.domElement.removeEventListener('pointercancel', pointerCancel);
            renderer.domElement.removeEventListener('webglcontextlost', contextLost);
            renderer.domElement.removeEventListener('wheel', wheelZoom);
        });
        controls.update();
        resize();

        return {
            update(items: GroupChallenge[], selectedID: string | null) {
                if (disposed) return;
                if (items !== previousItems) {
                    previousItems = items;
                    visible = items.filter((item) => Number.isFinite(item.lat) && Number.isFinite(item.long));
                    pinIndexes.clear();
                    selectedIndex = undefined;
                    positions = visible.map((item) => globePosition(item.lat!, item.long!, 1.016));
                    pinArtwork = visible.map((item, index) => {
                        pinIndexes.set(item.photo_id, index);
                        return getPinArtwork(item.map_pin?.image_url ?? DEFAULT_MAP_PIN_IMAGE_URL);
                    });
                    pinScreenPositions = new Float32Array(visible.length * 2);
                    pinScreenVisible = new Uint8Array(visible.length);
                }
                selectedIndex = selectedID === null ? undefined : pinIndexes.get(selectedID);
                syncPinOverlay();
                render();
            },
            focus(item: GroupChallenge) {
                if (item.lat === undefined || item.long === undefined) return;
                camera.position.copy(globePosition(item.lat, item.long, camera.position.length()));
                syncControls();
                controls.update();
            },
            rotate(horizontal: number, vertical: number) {
                const spherical = new THREE.Spherical().setFromVector3(camera.position);
                spherical.theta += horizontal / camera.zoom;
                spherical.phi = THREE.MathUtils.clamp(
                    spherical.phi + vertical / camera.zoom,
                    POLE_MARGIN,
                    Math.PI - POLE_MARGIN,
                );
                camera.position.setFromSpherical(spherical);
                syncControls();
                controls.update();
            },
            zoom(factor: number) {
                applyZoom(1 / factor);
            },
            controls,
            dispose,
        };
    } catch (error) {
        dispose();
        throw error;
    }
}
