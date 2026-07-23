import * as THREE from 'three';
import type { FaceFrame } from './facePose';
import type { LensId } from './lensCatalog';

type DeformationSource = HTMLVideoElement | HTMLCanvasElement;

const vertexShader = `
varying vec2 vUv;

void main() {
    vUv = uv;
    gl_Position = vec4(position.xy, 0.0, 1.0);
}
`;

const fragmentShader = `
uniform sampler2D source;
uniform vec2 faceCenter;
uniform vec2 faceRadius;
uniform vec2 leftEye;
uniform vec2 rightEye;
uniform vec2 mouthCenter;
uniform vec2 eyeRadius;
uniform int effect;
varying vec2 vUv;

vec2 bulge(vec2 uv, vec2 center, vec2 radius, float strength) {
    vec2 offset = (uv - center) / radius;
    float distanceFromCenter = length(offset);
    if (distanceFromCenter >= 1.0) return uv;
    float falloff = pow(1.0 - smoothstep(0.0, 1.0, distanceFromCenter), 1.35);
    return center + offset * radius * (1.0 - strength * falloff);
}

vec2 contract(vec2 uv, vec2 center, vec2 radius, float strength) {
    vec2 offset = (uv - center) / radius;
    float distanceFromCenter = length(offset);
    if (distanceFromCenter >= 1.0) return uv;
    float falloff = 1.0 - smoothstep(0.48, 1.0, distanceFromCenter);
    return center + offset * radius * (1.0 + strength * falloff);
}

void main() {
    vec2 cameraUv = vec2(vUv.x, 1.0 - vUv.y);
    vec2 sampleUv = cameraUv;
    if (effect == 1) {
        sampleUv = bulge(sampleUv, faceCenter, faceRadius * vec2(1.28, 1.2), 0.68);
    } else if (effect == 2) {
        sampleUv = bulge(sampleUv, leftEye, eyeRadius, 0.82);
        sampleUv = bulge(sampleUv, rightEye, eyeRadius, 0.82);
        sampleUv = bulge(sampleUv, mouthCenter, eyeRadius * vec2(1.35, 0.9), 0.38);
    } else if (effect == 3) {
        sampleUv = contract(sampleUv, faceCenter, faceRadius * vec2(1.22, 1.16), 1.12);
    }
    gl_FragColor = texture2D(source, clamp(sampleUv, vec2(0.002), vec2(0.998)));
}
`;

function effectIndex(id: LensId): number {
    if (id === 'big-head') return 1;
    if (id === 'bug-eyes') return 2;
    if (id === 'tiny-face') return 3;
    return 0;
}

export class FaceDeformation {
    readonly mesh: THREE.Mesh<THREE.PlaneGeometry, THREE.ShaderMaterial>;
    private texture: THREE.Texture | null = null;

    constructor() {
        const material = new THREE.ShaderMaterial({
            uniforms: {
                source: { value: null },
                faceCenter: { value: new THREE.Vector2(0.5, 0.5) },
                faceRadius: { value: new THREE.Vector2(0.25, 0.3) },
                leftEye: { value: new THREE.Vector2(0.4, 0.42) },
                rightEye: { value: new THREE.Vector2(0.6, 0.42) },
                mouthCenter: { value: new THREE.Vector2(0.5, 0.65) },
                eyeRadius: { value: new THREE.Vector2(0.08, 0.065) },
                effect: { value: 0 },
            },
            vertexShader,
            fragmentShader,
            transparent: false,
            depthWrite: false,
            depthTest: false,
        });
        this.mesh = new THREE.Mesh(new THREE.PlaneGeometry(2, 2), material);
        this.mesh.renderOrder = -100;
        this.mesh.frustumCulled = false;
        this.mesh.visible = false;
    }

    setSource(source: DeformationSource): void {
        this.texture?.dispose();
        this.texture =
            source instanceof HTMLVideoElement ? new THREE.VideoTexture(source) : new THREE.CanvasTexture(source);
        this.texture.colorSpace = THREE.SRGBColorSpace;
        this.texture.minFilter = THREE.LinearFilter;
        this.texture.magFilter = THREE.LinearFilter;
        this.texture.generateMipmaps = false;
        this.texture.flipY = false;
        this.mesh.material.uniforms.source.value = this.texture;
    }

    resize(width: number, height: number): void {
        this.mesh.userData.sourceSize = { width, height };
    }

    update(id: LensId, frame: FaceFrame | null): void {
        const effect = effectIndex(id);
        const landmarks = frame?.landmarks;
        this.mesh.visible = effect > 0 && Boolean(this.texture) && Boolean(landmarks && landmarks.length >= 468);
        this.mesh.material.uniforms.effect.value = this.mesh.visible ? effect : 0;
        if (!this.mesh.visible || !landmarks) return;
        if (this.texture instanceof THREE.CanvasTexture) this.texture.needsUpdate = true;

        const forehead = landmarks[10];
        const chin = landmarks[152];
        const leftCheek = landmarks[234];
        const rightCheek = landmarks[454];
        const centerX = (leftCheek.x + rightCheek.x) / 2;
        const centerY = (forehead.y + chin.y) / 2;
        const radiusX = Math.max(0.08, Math.abs(rightCheek.x - leftCheek.x) / 2);
        const radiusY = Math.max(0.1, Math.abs(chin.y - forehead.y) / 2);
        this.mesh.material.uniforms.faceCenter.value.set(centerX, centerY);
        this.mesh.material.uniforms.faceRadius.value.set(radiusX, radiusY);
        this.mesh.material.uniforms.leftEye.value.set(
            (landmarks[33].x + landmarks[133].x) / 2,
            (landmarks[33].y + landmarks[133].y) / 2,
        );
        this.mesh.material.uniforms.rightEye.value.set(
            (landmarks[362].x + landmarks[263].x) / 2,
            (landmarks[362].y + landmarks[263].y) / 2,
        );
        this.mesh.material.uniforms.mouthCenter.value.set(
            (landmarks[13].x + landmarks[14].x) / 2,
            (landmarks[13].y + landmarks[14].y) / 2,
        );
        this.mesh.material.uniforms.eyeRadius.value.set(radiusX * 0.34, radiusY * 0.23);
    }

    clear(): void {
        this.mesh.visible = false;
    }

    dispose(): void {
        this.texture?.dispose();
        this.mesh.geometry.dispose();
        this.mesh.material.dispose();
    }
}
