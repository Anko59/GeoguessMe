import * as THREE from 'three';
import type { FacePose } from './facePose';
import type { BuiltLens } from './lensBuilders';

const textureLoader = new THREE.TextureLoader();
const geometryLoader = new THREE.BufferGeometryLoader();

function loadColorTexture(path: string): THREE.Texture {
    const texture = import.meta.env.MODE === 'test' ? new THREE.Texture() : textureLoader.load(path);
    texture.colorSpace = THREE.SRGBColorSpace;
    texture.minFilter = THREE.LinearFilter;
    texture.magFilter = THREE.LinearFilter;
    texture.generateMipmaps = false;
    // The orthographic camera uses a top-left origin. Three's default image
    // upload flip would invert flat accessories a second time.
    texture.flipY = false;
    return texture;
}

function buildImageAccessory(
    path: string,
    width: number,
    height: number,
    y: number,
    sway: number,
    pulse = 0,
): BuiltLens {
    const root = new THREE.Group();
    const material = new THREE.MeshBasicMaterial({
        map: loadColorTexture(path),
        transparent: true,
        depthWrite: false,
        toneMapped: false,
        side: THREE.DoubleSide,
    });
    const headpiece = new THREE.Mesh(new THREE.PlaneGeometry(width, height), material);
    headpiece.position.set(0, y, 0.24);
    headpiece.renderOrder = 20;
    root.add(headpiece);

    return {
        root,
        update: (elapsed) => {
            headpiece.rotation.z = Math.sin(elapsed * 1.4) * sway;
            headpiece.position.y = y + Math.sin(elapsed * 1.9) * 0.012;
            const scale = 1 + Math.sin(elapsed * 2.2) * pulse;
            headpiece.scale.setScalar(scale);
        },
    };
}

function addDogPart(
    root: THREE.Group,
    geometryPath: string,
    texturePath: string,
    scale: [number, number, number],
    position: [number, number, number],
    alphaPath?: string,
): void {
    if (import.meta.env.MODE === 'test') {
        root.add(new THREE.Mesh(new THREE.BufferGeometry(), new THREE.MeshStandardMaterial()));
        return;
    }
    geometryLoader.load(geometryPath, (geometry) => {
        const material = new THREE.MeshStandardMaterial({
            map: loadColorTexture(texturePath),
            alphaMap: alphaPath ? loadColorTexture(alphaPath) : null,
            transparent: Boolean(alphaPath),
            roughness: 0.72,
            metalness: 0,
            side: THREE.DoubleSide,
        });
        const mesh = new THREE.Mesh(geometry, material);
        mesh.scale.set(...scale);
        mesh.position.set(...position);
        mesh.frustumCulled = false;
        mesh.renderOrder = 18;
        root.add(mesh);
    });
}

export function buildJeelizPuppy(): BuiltLens {
    const root = new THREE.Group();
    const base = '/lenses/jeeliz-dog';
    addDogPart(
        root,
        `${base}/dog_ears.geometry`,
        `${base}/texture_ears.jpg`,
        [0.016, -0.016, 0.016],
        [0, 0.24, 0],
        `${base}/alpha_ears_256.jpg`,
    );
    addDogPart(root, `${base}/dog_nose.geometry`, `${base}/texture_nose.jpg`, [0.018, -0.018, 0.018], [0, 0.24, 0.15]);

    const tongue = new THREE.Mesh(
        new THREE.CapsuleGeometry(0.055, 0.18, 8, 16),
        new THREE.MeshStandardMaterial({ color: '#f47f9f', roughness: 0.64 }),
    );
    tongue.position.set(0, 0.43, 0.19);
    tongue.rotation.z = Math.PI;
    root.add(tongue);

    return {
        root,
        update: (_elapsed: number, pose: FacePose) => {
            tongue.visible = pose.mouthOpen > 0.18;
            tongue.scale.y = 0.42 + pose.mouthOpen * 2.25;
            tongue.position.y = 0.39 + pose.mouthOpen * 0.15;
        },
    };
}

export function buildDiscoOutlaw(): BuiltLens {
    return buildImageAccessory('/lenses/generated/disco-outlaw.webp', 1.65, 1.1, -0.63, 0.018);
}

export function buildRedFlagRoyalty(): BuiltLens {
    return buildImageAccessory('/lenses/generated/red-flag-royalty.webp', 1.55, 1.55, -0.72, 0.012);
}

export function buildBadDecisions(): BuiltLens {
    return buildImageAccessory('/lenses/generated/bad-decisions.webp', 1.62, 1.08, -0.61, 0.022);
}

export function buildHrNightmare(): BuiltLens {
    return buildImageAccessory('/lenses/generated/hr-nightmare.webp', 2.05, 2.05, 0.12, 0.012, 0.012);
}

export function buildToxicEx(): BuiltLens {
    return buildImageAccessory('/lenses/generated/toxic-ex.webp', 2.02, 2.02, 0.05, -0.014, 0.018);
}

export function buildTaxFraud(): BuiltLens {
    return buildImageAccessory('/lenses/generated/tax-fraud.webp', 2.08, 2.08, 0.13, 0.01, 0.009);
}
