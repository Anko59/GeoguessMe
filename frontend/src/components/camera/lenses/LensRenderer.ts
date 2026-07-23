import * as THREE from 'three';
import { FaceDeformation } from './faceDeformation';
import { deriveFacePose, smoothFacePose, type FaceFrame, type FacePose } from './facePose';
import { buildLens, type BuiltLens } from './lensBuilders';
import type { LensId } from './lensCatalog';

const MAX_RENDER_EDGE = 1280;

function disposeObject(object: THREE.Object3D): void {
    object.traverse((child) => {
        if (!(child instanceof THREE.Mesh)) return;
        child.geometry.dispose();
        const materials = Array.isArray(child.material) ? child.material : [child.material];
        materials.forEach((material) => {
            Object.values(material).forEach((value) => {
                if (value instanceof THREE.Texture) value.dispose();
            });
            material.dispose();
        });
    });
}

export class LensRenderer {
    private readonly renderer: THREE.WebGLRenderer;
    private readonly scene = new THREE.Scene();
    private readonly camera: THREE.OrthographicCamera;
    private builtLens: BuiltLens = buildLens('none');
    private selectedLens: LensId = 'none';
    private smoothedPose: FacePose | null = null;
    private readonly deformation = new FaceDeformation();
    private width = 1;
    private height = 1;
    private readonly startedAt = performance.now();

    constructor(canvas: HTMLCanvasElement) {
        this.renderer = new THREE.WebGLRenderer({
            canvas,
            alpha: true,
            antialias: true,
            premultipliedAlpha: true,
            preserveDrawingBuffer: true,
            powerPreference: 'high-performance',
        });
        this.renderer.setClearColor(0x000000, 0);
        this.renderer.outputColorSpace = THREE.SRGBColorSpace;
        this.camera = new THREE.OrthographicCamera(0, 1, 0, 1, -2000, 2000);
        this.camera.position.set(0, 0, 1000);
        this.camera.lookAt(0, 0, 0);
        this.scene.add(new THREE.HemisphereLight(0xffffff, 0x4050a0, 2.4));
        const key = new THREE.DirectionalLight(0xffffff, 3.4);
        key.position.set(-400, -500, 800);
        this.scene.add(key);
        const rim = new THREE.DirectionalLight(0xff68be, 2.1);
        rim.position.set(500, 100, 500);
        this.scene.add(rim);
        this.scene.add(this.deformation.mesh);
        this.scene.add(this.builtLens.root);
    }

    setSource(source: HTMLVideoElement | HTMLCanvasElement): void {
        this.deformation.setSource(source);
    }

    resize(sourceWidth: number, sourceHeight: number): void {
        const scale = Math.min(1, MAX_RENDER_EDGE / Math.max(sourceWidth, sourceHeight));
        this.width = Math.max(1, Math.round(sourceWidth * scale));
        this.height = Math.max(1, Math.round(sourceHeight * scale));
        this.renderer.setSize(this.width, this.height, false);
        this.deformation.resize(this.width, this.height);
        this.camera.left = 0;
        this.camera.right = this.width;
        this.camera.top = 0;
        this.camera.bottom = this.height;
        this.camera.updateProjectionMatrix();
    }

    setLens(id: LensId): void {
        if (id === this.selectedLens) return;
        this.scene.remove(this.builtLens.root);
        disposeObject(this.builtLens.root);
        this.selectedLens = id;
        this.builtLens = buildLens(id);
        this.scene.add(this.builtLens.root);
    }

    render(frame: FaceFrame | null, timestamp = performance.now()): boolean {
        this.renderer.clear();
        this.deformation.update(this.selectedLens, frame);
        if (this.selectedLens === 'none') {
            this.builtLens.root.visible = false;
            this.renderer.render(this.scene, this.camera);
            return true;
        }

        const pose = deriveFacePose(frame ?? { landmarks: [], blendshapes: {} }, this.width, this.height);
        this.smoothedPose = smoothFacePose(this.smoothedPose, pose);
        if (!this.smoothedPose) {
            this.builtLens.root.visible = false;
            this.renderer.render(this.scene, this.camera);
            return false;
        }

        const current = this.smoothedPose;
        const root = this.builtLens.root;
        root.visible = true;
        root.position.set(current.centerX, current.centerY, 0);
        root.scale.setScalar(current.faceWidth);
        root.rotation.set(current.pitch * 0.48, -current.yaw, current.roll);
        this.builtLens.update((timestamp - this.startedAt) / 1000, current);
        this.renderer.render(this.scene, this.camera);
        return true;
    }

    clear(): void {
        this.smoothedPose = null;
        this.deformation.clear();
        this.renderer.clear();
    }

    dispose(): void {
        disposeObject(this.builtLens.root);
        this.deformation.dispose();
        this.renderer.dispose();
    }
}
