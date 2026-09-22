import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import type { GroupChallenge } from '../../types';
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
        let syncPinScale = () => {};
        const syncDetailTiles = () => {
            syncPinScale();
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
        const pinGeometry = new THREE.SphereGeometry(1, 10, 8);
        cleanup.push(() => pinGeometry.dispose());
        // Instanced colors are only consumed by Three.js materials with
        // vertex colors enabled. Without this flag, the CPU-side colors set
        // below never reach the shader and selected pins cannot be highlighted.
        const pinMaterial = new THREE.MeshBasicMaterial({ vertexColors: true });
        cleanup.push(() => pinMaterial.dispose());
        let pins = new THREE.InstancedMesh(pinGeometry, pinMaterial, 0);
        cleanup.push(() => pins.dispose());
        let visible: GroupChallenge[] = [];
        let positions: THREE.Vector3[] = [];
        let previousItems: GroupChallenge[] | undefined;
        let selectedIndex: number | undefined;
        const pinIndexes = new Map<string, number>();
        scene.add(pins);
        const cameraPosition = new THREE.Vector3();
        const screenScale = new THREE.Vector3();
        const pinMatrix = new THREE.Matrix4();
        syncPinScale = () => {
            if (host.clientHeight <= 0) return;
            camera.updateMatrixWorld();
            for (let index = 0; index < positions.length; index += 1) {
                cameraPosition.copy(positions[index]).applyMatrix4(camera.matrixWorldInverse);
                const depth = Math.max(-cameraPosition.z, 0.1);
                const diameter = index === selectedIndex ? 8 : 6;
                const radius =
                    (diameter * depth * Math.tan(THREE.MathUtils.degToRad(camera.getEffectiveFOV() / 2))) /
                    host.clientHeight;
                screenScale.setScalar(radius);
                pinMatrix.compose(positions[index], new THREE.Quaternion(), screenScale);
                pins.setMatrixAt(index, pinMatrix);
            }
            pins.instanceMatrix.needsUpdate = true;
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
            syncControls();
            syncPinScale();
            render();
            detailLayer.scheduleUpdate();
        };
        const observer = new ResizeObserver(resize);
        cleanup.push(() => observer.disconnect());
        observer.observe(host);
        const raycaster = new THREE.Raycaster();
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
            syncPinScale();
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
            raycaster.setFromCamera(
                new THREE.Vector2(
                    ((event.clientX - bounds.left) / bounds.width) * 2 - 1,
                    (-(event.clientY - bounds.top) / bounds.height) * 2 + 1,
                ),
                camera,
            );
            // The Earth participates in hit testing so far-side pins cannot be picked.
            const hit = raycaster.intersectObjects([earth, pins], false)[0];
            if (hit?.object === pins && hit.instanceId !== undefined) {
                onSelect(visible[hit.instanceId].photo_id);
                return;
            }
            // Pins stay visually small, but a finger can select the nearest
            // front-facing pin within a 44px target. Never include far-side pins.
            let nearest = event.pointerType === 'touch' ? 22 : 12;
            let candidate: number | undefined;
            const projected = new THREE.Vector3();
            positions.forEach((position, index) => {
                if (position.dot(camera.position) <= position.lengthSq()) return;
                projected.copy(position).project(camera);
                if (Math.abs(projected.x) > 1 || Math.abs(projected.y) > 1) return;
                const x = bounds.left + ((projected.x + 1) * bounds.width) / 2;
                const y = bounds.top + ((1 - projected.y) * bounds.height) / 2;
                const distance = Math.hypot(event.clientX - x, event.clientY - y);
                if (distance < nearest) {
                    nearest = distance;
                    candidate = index;
                }
            });
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
                    if (visible.length > pins.instanceMatrix.count) {
                        scene.remove(pins);
                        pins.dispose();
                        // Grow geometrically while loading pages; selection reuses GPU buffers.
                        pins = new THREE.InstancedMesh(
                            pinGeometry,
                            pinMaterial,
                            Math.max(visible.length, pins.instanceMatrix.count * 2),
                        );
                        scene.add(pins);
                    }
                    pins.count = visible.length;
                    pinIndexes.clear();
                    selectedIndex = undefined;
                    const matrix = new THREE.Matrix4();
                    const color = new THREE.Color('#ffb638');
                    positions = visible.map((item) => globePosition(item.lat!, item.long!, 1.016));
                    visible.forEach((item, index) => {
                        const position = positions[index];
                        pins.setMatrixAt(index, matrix.makeTranslation(position.x, position.y, position.z));
                        pins.setColorAt(index, color);
                        pinIndexes.set(item.photo_id, index);
                    });
                    pins.instanceMatrix.needsUpdate = true;
                    pins.computeBoundingSphere();
                }
                if (selectedIndex !== undefined) pins.setColorAt(selectedIndex, new THREE.Color('#ffb638'));
                selectedIndex = selectedID === null ? undefined : pinIndexes.get(selectedID);
                if (selectedIndex !== undefined) pins.setColorAt(selectedIndex, new THREE.Color('#ffffff'));
                if (pins.instanceColor) pins.instanceColor.needsUpdate = true;
                syncPinScale();
                render();
            },
            focus(item: GroupChallenge) {
                if (item.lat === undefined || item.long === undefined) return;
                camera.position.copy(globePosition(item.lat, item.long, camera.position.length()));
                syncControls();
                controls.update();
                syncPinScale();
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
                syncPinScale();
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
