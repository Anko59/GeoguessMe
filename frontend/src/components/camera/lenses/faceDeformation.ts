import * as THREE from 'three';
import type { FaceFrame } from './facePose';
import type { LensId } from './lensCatalog';

type DeformationSource = HTMLVideoElement | HTMLCanvasElement;

const vertexShader = `
varying vec2 vUv;

void main() {
    vUv = uv;
    gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const fragmentShader = `
uniform sampler2D source;
uniform vec2 faceCenter;
uniform vec2 leftEye;
uniform vec2 rightEye;
uniform float aspect;
uniform int effect;
varying vec2 vUv;

vec2 warp(vec2 uv, vec2 center, float radius, float strength) {
    vec2 offset = uv - center;
    vec2 measured = vec2(offset.x * aspect, offset.y);
    float distanceFromCenter = length(measured);
    if (distanceFromCenter >= radius) return uv;
    float falloff = 1.0 - distanceFromCenter / radius;
    return center + offset * (1.0 - strength * falloff * falloff);
}

void main() {
    vec2 cameraUv = vec2(vUv.x, 1.0 - vUv.y);
    vec2 sampleUv = cameraUv;
    if (effect == 1) {
        sampleUv = warp(sampleUv, faceCenter, 0.42, 0.48);
    } else if (effect == 2) {
        sampleUv = warp(sampleUv, leftEye, 0.105, 0.64);
        sampleUv = warp(sampleUv, rightEye, 0.105, 0.64);
    } else if (effect == 3) {
        sampleUv = warp(sampleUv, faceCenter, 0.44, -0.58);
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
                leftEye: { value: new THREE.Vector2(0.36, 0.42) },
                rightEye: { value: new THREE.Vector2(0.64, 0.42) },
                aspect: { value: 1 },
                effect: { value: 0 },
            },
            vertexShader,
            fragmentShader,
            transparent: false,
            depthWrite: false,
            depthTest: false,
        });
        this.mesh = new THREE.Mesh(new THREE.PlaneGeometry(1, 1), material);
        this.mesh.position.z = -100;
        this.mesh.renderOrder = -100;
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
        this.mesh.scale.set(width, height, 1);
        this.mesh.position.set(width / 2, height / 2, -100);
        this.mesh.material.uniforms.aspect.value = width / height;
    }

    update(id: LensId, frame: FaceFrame | null): void {
        const effect = effectIndex(id);
        this.mesh.visible = effect > 0 && Boolean(this.texture);
        this.mesh.material.uniforms.effect.value = frame ? effect : 0;
        if (!this.mesh.visible) return;
        if (this.texture instanceof THREE.CanvasTexture) this.texture.needsUpdate = true;
        if (!frame || frame.landmarks.length < 468) return;

        const landmarks = frame.landmarks;
        const forehead = landmarks[10];
        const chin = landmarks[152];
        const leftCheek = landmarks[234];
        const rightCheek = landmarks[454];
        this.mesh.material.uniforms.faceCenter.value.set((leftCheek.x + rightCheek.x) / 2, (forehead.y + chin.y) / 2);
        this.mesh.material.uniforms.leftEye.value.set(landmarks[33].x, landmarks[33].y);
        this.mesh.material.uniforms.rightEye.value.set(landmarks[263].x, landmarks[263].y);
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
