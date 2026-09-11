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
    const renderer = new THREE.WebGLRenderer({ antialias: true, alpha: true });
    renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
    renderer.domElement.setAttribute('aria-hidden', 'true');
    host.appendChild(renderer.domElement);
    const scene = new THREE.Scene();
    const camera = new THREE.PerspectiveCamera(42, 1, 0.1, 50);
    camera.position.copy(globePosition(22, 12, 3.5));
    const controls = new OrbitControls(camera, renderer.domElement);
    controls.enablePan = false;
    controls.minDistance = 1.6;
    controls.maxDistance = 6;
    // Render only on interaction: no idle animation or motion preference override.
    controls.enableDamping = false;
    const render = () => renderer.render(scene, camera);
    controls.addEventListener('change', render);
    const earthGeometry = new THREE.SphereGeometry(1, 64, 48);
    const earthMaterial = new THREE.MeshPhongMaterial({ color: 0xb9d9f5, shininess: 8 });
    const earth = new THREE.Mesh(earthGeometry, earthMaterial);
    scene.add(earth);
    scene.add(new THREE.AmbientLight(0xffffff, 2));
    const light = new THREE.DirectionalLight(0xffffff, 2);
    light.position.set(-3, 5, 4);
    scene.add(light);
    let disposed = false;
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
    const pinGeometry = new THREE.SphereGeometry(0.018, 10, 8);
    const pinMaterial = new THREE.MeshBasicMaterial();
    let pins = new THREE.InstancedMesh(pinGeometry, pinMaterial, 0);
    let visible: GroupChallenge[] = [];
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
        if (hit?.object === pins && hit.instanceId !== undefined) onSelect(visible[hit.instanceId].photo_id);
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
    controls.update();
    resize();

    return {
        update(items: GroupChallenge[], selectedID: string | null) {
            visible = items.filter((item) => Number.isFinite(item.lat) && Number.isFinite(item.long));
            scene.remove(pins);
            pins.dispose();
            pins = new THREE.InstancedMesh(pinGeometry, pinMaterial, visible.length);
            visible.forEach((item, index) => {
                const position = globePosition(item.lat!, item.long!, 1.016);
                pins.setMatrixAt(index, new THREE.Matrix4().makeTranslation(position.x, position.y, position.z));
                pins.setColorAt(index, new THREE.Color(item.photo_id === selectedID ? '#ffffff' : '#ffb638'));
            });
            pins.instanceMatrix.needsUpdate = true;
            scene.add(pins);
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
                THREE.MathUtils.clamp(camera.position.length() * factor, controls.minDistance, controls.maxDistance),
            );
            controls.update();
        },
        dispose() {
            disposed = true;
            observer.disconnect();
            controls.removeEventListener('change', render);
            controls.dispose();
            renderer.domElement.removeEventListener('pointerdown', pointerDown);
            renderer.domElement.removeEventListener('pointermove', pointerMove);
            renderer.domElement.removeEventListener('pointerup', pointerUp);
            renderer.domElement.removeEventListener('pointercancel', pointerCancel);
            renderer.domElement.removeEventListener('webglcontextlost', contextLost);
            texture.dispose();
            earthGeometry.dispose();
            earthMaterial.dispose();
            pins.dispose();
            pinGeometry.dispose();
            pinMaterial.dispose();
            renderer.dispose();
            renderer.forceContextLoss();
            renderer.domElement.remove();
        },
    };
}
