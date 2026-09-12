import * as THREE from 'three';
import { OrbitControls } from 'three/addons/controls/OrbitControls.js';
import type { GroupChallenge } from '../../types';

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

export function createGlobeScene(
    host: HTMLDivElement,
    onSelect: (id: string) => void,
    onError: (message: string) => void,
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
        renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
        renderer.domElement.setAttribute('aria-hidden', 'true');
        host.appendChild(renderer.domElement);
        const scene = new THREE.Scene();
        const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 50);
        camera.position.copy(globePosition(22, 12, 3.5));
        const controls = new OrbitControls(camera, renderer.domElement);
        cleanup.push(() => controls.dispose());
        controls.enablePan = false;
        controls.minDistance = 1.6;
        controls.maxDistance = 6;
        // Render only on interaction: no idle animation or motion preference override.
        controls.enableDamping = false;
        const render = () => renderer.render(scene, camera);
        controls.addEventListener('change', render);
        cleanup.push(() => controls.removeEventListener('change', render));
        const earthGeometry = new THREE.SphereGeometry(1, 64, 48);
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
            '/globe/earth.jpg',
            (loaded) => {
                if (disposed) return;
                loaded.colorSpace = THREE.SRGBColorSpace;
                earthMaterial.map = loaded;
                earthMaterial.color.set(0xffffff);
                earthMaterial.needsUpdate = true;
                render();
            },
            undefined,
            () => {
                if (!disposed) onError('The Earth texture could not load. Your challenge list is still available.');
            },
        );
        cleanup.push(() => texture.dispose());
        const pinGeometry = new THREE.SphereGeometry(0.018, 10, 8);
        cleanup.push(() => pinGeometry.dispose());
        const pinMaterial = new THREE.MeshBasicMaterial();
        cleanup.push(() => pinMaterial.dispose());
        let pins = new THREE.InstancedMesh(pinGeometry, pinMaterial, 0);
        cleanup.push(() => pins.dispose());
        let visible: GroupChallenge[] = [];
        let positions: THREE.Vector3[] = [];
        let previousItems: GroupChallenge[] | undefined;
        let selectedIndex: number | undefined;
        const pinIndexes = new Map<string, number>();
        scene.add(pins);
        const resize = () => {
            const width = Math.max(host.clientWidth, 1);
            const height = Math.max(host.clientHeight, 1);
            camera.aspect = width / height;
            camera.updateProjectionMatrix();
            renderer.setSize(width, height);
            render();
        };
        const observer = new ResizeObserver(resize);
        cleanup.push(() => observer.disconnect());
        observer.observe(host);
        const raycaster = new THREE.Raycaster();
        let pointerStart: { x: number; y: number } | null = null;
        let dragged = false;
        const pointerDown = (event: PointerEvent) => {
            if (event.button !== 0) return;
            if (pointerStart) {
                dragged = true;
                return;
            }
            pointerStart = { x: event.clientX, y: event.clientY };
            dragged = false;
        };
        const pointerCancel = () => {
            pointerStart = null;
        };
        const pointerMove = (event: PointerEvent) => {
            if (pointerStart && Math.hypot(event.clientX - pointerStart.x, event.clientY - pointerStart.y) > 6)
                dragged = true;
        };
        const pointerUp = (event: PointerEvent) => {
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
        renderer.domElement.addEventListener('pointerdown', pointerDown);
        renderer.domElement.addEventListener('pointermove', pointerMove);
        renderer.domElement.addEventListener('pointerup', pointerUp);
        renderer.domElement.addEventListener('pointercancel', pointerCancel);
        renderer.domElement.addEventListener('webglcontextlost', contextLost);
        cleanup.push(() => {
            renderer.domElement.removeEventListener('pointerdown', pointerDown);
            renderer.domElement.removeEventListener('pointermove', pointerMove);
            renderer.domElement.removeEventListener('pointerup', pointerUp);
            renderer.domElement.removeEventListener('pointercancel', pointerCancel);
            renderer.domElement.removeEventListener('webglcontextlost', contextLost);
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
                render();
            },
            focus(item: GroupChallenge) {
                if (item.lat === undefined || item.long === undefined) return;
                camera.position.copy(globePosition(item.lat, item.long, camera.position.length()));
                controls.update();
            },
            rotate(horizontal: number, vertical: number) {
                const spherical = new THREE.Spherical().setFromVector3(camera.position);
                spherical.theta += horizontal;
                spherical.phi = THREE.MathUtils.clamp(spherical.phi + vertical, 0.05, Math.PI - 0.05);
                camera.position.setFromSpherical(spherical);
                controls.update();
            },
            zoom(factor: number) {
                camera.position.setLength(
                    THREE.MathUtils.clamp(
                        camera.position.length() * factor,
                        controls.minDistance,
                        controls.maxDistance,
                    ),
                );
                controls.update();
            },
            dispose,
        };
    } catch (error) {
        dispose();
        throw error;
    }
}
