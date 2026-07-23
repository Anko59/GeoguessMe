import * as THREE from 'three';
import { buildBadDecisions, buildDiscoOutlaw, buildJeelizPuppy, buildRedFlagRoyalty } from './assetLensBuilders';
import type { FacePose } from './facePose';
import type { LensId } from './lensCatalog';

export interface BuiltLens {
    root: THREE.Group;
    update: (elapsed: number, pose: FacePose) => void;
}

type Material = THREE.MeshStandardMaterial | THREE.MeshPhysicalMaterial;

const standard = (
    color: THREE.ColorRepresentation,
    emissive: THREE.ColorRepresentation = 0x000000,
): THREE.MeshStandardMaterial => new THREE.MeshStandardMaterial({ color, emissive, roughness: 0.32, metalness: 0.28 });

const glass = (color: THREE.ColorRepresentation, opacity = 0.42): THREE.MeshPhysicalMaterial =>
    new THREE.MeshPhysicalMaterial({
        color,
        emissive: color,
        emissiveIntensity: 0.28,
        roughness: 0.08,
        metalness: 0.18,
        transparent: true,
        opacity,
        transmission: 0.35,
        thickness: 0.08,
        side: THREE.DoubleSide,
    });

function addMesh(
    parent: THREE.Object3D,
    geometry: THREE.BufferGeometry,
    material: Material,
    position: [number, number, number] = [0, 0, 0],
    scale: [number, number, number] = [1, 1, 1],
    rotation: [number, number, number] = [0, 0, 0],
): THREE.Mesh<THREE.BufferGeometry, Material> {
    const item = new THREE.Mesh<THREE.BufferGeometry, Material>(geometry, material);
    item.position.set(...position);
    item.scale.set(...scale);
    item.rotation.set(...rotation);
    parent.add(item);
    return item;
}

const sphere = (parent: THREE.Object3D, material: Material, position: [number, number, number], scale: number[]) =>
    addMesh(parent, new THREE.SphereGeometry(0.12, 24, 16), material, position, [scale[0], scale[1], scale[2]]);

const uprightCone = (
    parent: THREE.Object3D,
    material: Material,
    position: [number, number, number],
    radius: number,
    height: number,
    segments: number,
    scale: [number, number, number] = [1, 1, 1],
    tilt = 0,
) => addMesh(parent, new THREE.ConeGeometry(radius, height, segments), material, position, scale, [Math.PI, 0, tilt]);

function addFrame(parent: THREE.Object3D, color: THREE.ColorRepresentation, y = 0.02): THREE.Group {
    const frame = new THREE.Group();
    const material = standard(color, color);
    const lensGeometry = new THREE.TorusGeometry(0.205, 0.026, 12, 36);
    addMesh(frame, lensGeometry, material, [-0.235, y, 0.08], [1.18, 0.78, 1]);
    addMesh(frame, lensGeometry, material, [0.235, y, 0.08], [1.18, 0.78, 1]);
    addMesh(frame, new THREE.BoxGeometry(0.14, 0.028, 0.025), material, [0, y, 0.08]);
    addMesh(
        frame,
        new THREE.BoxGeometry(0.2, 0.025, 0.02),
        material,
        [-0.49, y - 0.02, 0.04],
        [1, 1, 1],
        [0, 0, -0.12],
    );
    addMesh(frame, new THREE.BoxGeometry(0.2, 0.025, 0.02), material, [0.49, y - 0.02, 0.04], [1, 1, 1], [0, 0, 0.12]);
    parent.add(frame);
    return frame;
}

function floatingParticles(
    parent: THREE.Object3D,
    colors: THREE.ColorRepresentation[],
    count: number,
    radius = 0.025,
): THREE.Mesh[] {
    const items: THREE.Mesh[] = [];
    for (let index = 0; index < count; index += 1) {
        const angle = (index / count) * Math.PI * 2;
        const item = addMesh(
            parent,
            new THREE.OctahedronGeometry(radius, 0),
            standard(colors[index % colors.length], colors[index % colors.length]),
            [Math.cos(angle) * (0.48 + (index % 3) * 0.08), Math.sin(angle) * 0.52, 0.12],
        );
        items.push(item);
    }
    return items;
}

function buildCyber(): BuiltLens {
    const root = new THREE.Group();
    const visor = addMesh(
        root,
        new THREE.BoxGeometry(0.9, 0.235, 0.055, 8, 3, 2),
        glass('#16e8ff', 0.5),
        [0, 0.02, 0.1],
    );
    visor.material.depthWrite = false;
    const edge = standard('#7bfbff', '#16e8ff');
    addMesh(root, new THREE.BoxGeometry(0.92, 0.025, 0.075), edge, [0, -0.105, 0.13]);
    addMesh(root, new THREE.BoxGeometry(0.92, 0.025, 0.075), edge, [0, 0.145, 0.13]);
    sphere(root, standard('#ff4fc8', '#ff4fc8'), [-0.5, 0.02, 0.12], [0.62, 0.62, 0.62]);
    sphere(root, standard('#ff4fc8', '#ff4fc8'), [0.5, 0.02, 0.12], [0.62, 0.62, 0.62]);
    const scan = addMesh(
        root,
        new THREE.BoxGeometry(0.015, 0.2, 0.01),
        standard('#ffffff', '#ffffff'),
        [-0.35, 0.02, 0.17],
    );
    return {
        root,
        update: (elapsed) => {
            scan.position.x = -0.36 + ((elapsed * 0.42) % 0.72);
            visor.material.opacity = 0.42 + Math.sin(elapsed * 4) * 0.06;
        },
    };
}

function buildCrown(): BuiltLens {
    const root = new THREE.Group();
    const gold = standard('#e9b949', '#6d3900');
    const band = addMesh(root, new THREE.BoxGeometry(0.86, 0.1, 0.08), gold, [0, -0.42, 0.06]);
    const gems: THREE.Mesh[] = [];
    const colors = ['#63e8ff', '#df76ff', '#ff5d93', '#75ffb0', '#ffe56b'];
    for (let index = 0; index < 5; index += 1) {
        const x = (index - 2) * 0.19;
        const height = index === 2 ? 0.4 : index % 2 === 0 ? 0.28 : 0.33;
        uprightCone(root, gold, [x, -0.46 - height / 2, 0.04], 0.105, height, 5);
        gems.push(
            addMesh(root, new THREE.OctahedronGeometry(index === 2 ? 0.09 : 0.065), glass(colors[index], 0.82), [
                x,
                -0.42 - height * 0.76,
                0.13,
            ]),
        );
    }
    return {
        root,
        update: (elapsed) => {
            band.rotation.y = Math.sin(elapsed * 0.8) * 0.035;
            gems.forEach((gem, index) => (gem.rotation.y = elapsed * (0.7 + index * 0.08)));
        },
    };
}

function buildCat(): BuiltLens {
    const root = new THREE.Group();
    const fur = standard('#292342', '#120622');
    const pink = standard('#ff75be', '#9b175e');
    for (const side of [-1, 1]) {
        uprightCone(root, fur, [side * 0.32, -0.46, 0.02], 0.2, 0.4, 3, [1, 1, 0.65]);
        uprightCone(root, pink, [side * 0.32, -0.45, 0.08], 0.11, 0.26, 3, [1, 1, 0.55]);
        for (let line = -1; line <= 1; line += 1) {
            addMesh(
                root,
                new THREE.BoxGeometry(0.42, 0.012, 0.012),
                standard('#fff0fb', '#ff73c5'),
                [side * 0.34, 0.31 + line * 0.06, 0.13],
                [1, 1, 1],
                [0, 0, side * line * 0.08],
            );
        }
    }
    const nose = addMesh(
        root,
        new THREE.ConeGeometry(0.07, 0.1, 3),
        pink,
        [0, 0.25, 0.16],
        [1, 1, 0.75],
        [Math.PI / 2, 0, Math.PI],
    );
    return { root, update: (elapsed) => (nose.rotation.z = Math.PI + Math.sin(elapsed * 3) * 0.08) };
}

function buildDevil(): BuiltLens {
    const root = new THREE.Group();
    const horn = standard('#6d1018', '#420008');
    for (const side of [-1, 1]) {
        uprightCone(root, horn, [side * 0.34, -0.47, 0.03], 0.13, 0.55, 24, [1, 1, 0.8], side * -0.34);
    }
    const embers = floatingParticles(root, ['#ff311f', '#ff8a24', '#ffd05b'], 12, 0.025);
    return {
        root,
        update: (elapsed) =>
            embers.forEach((ember, index) => {
                ember.position.y = 0.45 - ((elapsed * (0.18 + (index % 3) * 0.05) + index * 0.09) % 1.05);
                ember.position.x += Math.sin(elapsed * 2 + index) * 0.0008;
                ember.rotation.y += 0.04;
            }),
    };
}

function buildAngel(): BuiltLens {
    const root = new THREE.Group();
    const halo = addMesh(
        root,
        new THREE.TorusGeometry(0.37, 0.025, 16, 64),
        standard('#fff0a8', '#ffd44f'),
        [0, -0.63, 0.02],
        [1, 0.34, 1],
    );
    const feathers = standard('#f7fbff', '#8fbfff');
    for (const side of [-1, 1]) {
        for (let index = 0; index < 4; index += 1) {
            sphere(
                root,
                feathers,
                [side * (0.53 + index * 0.06), -0.06 + index * 0.07, -0.02],
                [1.45, 0.48, 0.3],
            ).rotation.z = side * (0.55 + index * 0.08);
        }
    }
    return { root, update: (elapsed) => (halo.position.y = -0.63 + Math.sin(elapsed * 1.7) * 0.025) };
}

function buildSpace(): BuiltLens {
    const root = new THREE.Group();
    const helmet = sphere(root, glass('#8ec8ff', 0.2), [0, 0.04, -0.05], [5.3, 6.1, 2.4]);
    helmet.material.depthWrite = false;
    addMesh(
        root,
        new THREE.TorusGeometry(0.57, 0.045, 16, 64),
        standard('#d7e5ff', '#477dff'),
        [0, 0.08, 0],
        [1, 1.12, 1],
    );
    const antenna = addMesh(root, new THREE.CylinderGeometry(0.012, 0.012, 0.35), standard('#becce3'), [0.42, -0.5, 0]);
    antenna.rotation.z = -0.25;
    const beacon = sphere(root, standard('#ff578b', '#ff1d68'), [0.46, -0.69, 0], [0.6, 0.6, 0.6]);
    return {
        root,
        update: (elapsed) => beacon.scale.setScalar(0.48 + (Math.sin(elapsed * 5) + 1) * 0.12),
    };
}

function buildParty(): BuiltLens {
    const root = new THREE.Group();
    uprightCone(root, standard('#8a50ff', '#4011b8'), [0.08, -0.65, 0.02], 0.3, 0.7, 32, [1, 1, 0.6]);
    addMesh(
        root,
        new THREE.TorusGeometry(0.28, 0.035, 12, 40),
        standard('#ffd64e', '#ff7a00'),
        [0.08, -0.31, 0.06],
        [1, 0.34, 1],
    );
    const pom = sphere(root, standard('#ff559f', '#ff166f'), [0.08, -1, 0.03], [0.85, 0.85, 0.85]);
    const confetti = floatingParticles(root, ['#ff4d89', '#ffe14f', '#40e8ff', '#8d6cff'], 18, 0.024);
    return {
        root,
        update: (elapsed) => {
            pom.rotation.y = elapsed * 2;
            confetti.forEach((piece, index) => {
                piece.position.y = -0.65 + ((elapsed * 0.28 + index * 0.11) % 1.25);
                piece.rotation.x += 0.05;
            });
        },
    };
}

function buildButterfly(): BuiltLens {
    const root = new THREE.Group();
    const wings: THREE.Mesh[] = [];
    for (const side of [-1, 1]) {
        wings.push(
            sphere(root, glass(side < 0 ? '#8e64ff' : '#ff77c8', 0.72), [side * 0.43, -0.1, 0.08], [2.2, 2.8, 0.18]),
            sphere(root, glass(side < 0 ? '#55d7ff' : '#ffbd5b', 0.68), [side * 0.5, 0.22, 0.06], [1.7, 2, 0.16]),
        );
    }
    sphere(root, standard('#44205f', '#8e4dba'), [0, 0.02, 0.14], [0.36, 2.5, 0.36]);
    return {
        root,
        update: (elapsed) =>
            wings.forEach((wing, index) => (wing.rotation.y = Math.sin(elapsed * 5 + index * Math.PI) * 0.28)),
    };
}

function buildFrog(): BuiltLens {
    const root = new THREE.Group();
    for (const side of [-1, 1]) {
        sphere(root, standard('#59c84f', '#174f19'), [side * 0.31, -0.24, 0.08], [1.5, 1.5, 0.8]);
        sphere(root, standard('#fffce5'), [side * 0.31, -0.25, 0.17], [0.8, 0.8, 0.4]);
        sphere(root, standard('#172017'), [side * 0.31, -0.25, 0.22], [0.34, 0.5, 0.25]);
    }
    const crown = uprightCone(root, standard('#ffd64d', '#b06000'), [0, -0.57, 0.05], 0.11, 0.3, 5);
    return { root, update: (elapsed) => (crown.rotation.y = elapsed * 0.8) };
}

function buildRobot(): BuiltLens {
    const root = new THREE.Group();
    addMesh(root, new THREE.BoxGeometry(0.88, 0.24, 0.08), glass('#35e7ff', 0.52), [0, 0.02, 0.1]);
    const metal = standard('#6f7f99', '#19334d');
    for (const side of [-1, 1]) {
        addMesh(
            root,
            new THREE.BoxGeometry(0.18, 0.34, 0.12),
            metal,
            [side * 0.5, 0.05, 0.04],
            [1, 1, 1],
            [0, 0, side * 0.16],
        );
        sphere(root, standard('#ff4f6d', '#ff123f'), [side * 0.5, 0.05, 0.13], [0.42, 0.42, 0.42]);
    }
    const meter = addMesh(
        root,
        new THREE.BoxGeometry(0.12, 0.018, 0.018),
        standard('#7cff91', '#2cff4e'),
        [0, 0.02, 0.18],
    );
    return { root, update: (elapsed) => (meter.scale.x = 0.45 + (Math.sin(elapsed * 4) + 1) * 0.28) };
}

function buildMasquerade(): BuiltLens {
    const root = new THREE.Group();
    const purple = standard('#682f91', '#26063d');
    for (const side of [-1, 1]) {
        sphere(root, purple, [side * 0.23, 0.03, 0.1], [2.25, 1.15, 0.25]);
        uprightCone(
            root,
            standard('#eabf55', '#7a3a00'),
            [side * 0.4, -0.34, 0.05],
            0.1,
            0.48,
            8,
            [0.7, 1, 0.5],
            side * -0.32,
        );
    }
    addFrame(root, '#f1ca68', 0.02).scale.set(0.82, 0.72, 1);
    return { root, update: () => undefined };
}

function buildIce(): BuiltLens {
    const root = new THREE.Group();
    const ice = glass('#8ceaff', 0.78);
    for (let index = 0; index < 7; index += 1) {
        const x = (index - 3) * 0.13;
        const height = 0.25 + (1 - Math.abs(index - 3) / 4) * 0.28;
        uprightCone(root, ice, [x, -0.39 - height / 2, 0.04], 0.075, height, 5);
    }
    const snow = floatingParticles(root, ['#ffffff', '#8ceaff'], 14, 0.018);
    return {
        root,
        update: (elapsed) =>
            snow.forEach((flake, index) => {
                flake.position.y = -0.7 + ((elapsed * 0.16 + index * 0.12) % 1.35);
                flake.rotation.z += 0.025;
            }),
    };
}

function buildArcade(): BuiltLens {
    const root = new THREE.Group();
    const dark = standard('#171827');
    for (const side of [-1, 1]) {
        for (let x = 0; x < 4; x += 1) {
            for (let y = 0; y < 3; y += 1) {
                if (y === 2 && (x === 0 || x === 3)) continue;
                addMesh(root, new THREE.BoxGeometry(0.085, 0.075, 0.035), dark, [
                    side * 0.24 + (x - 1.5) * 0.075,
                    -0.035 + y * 0.065,
                    0.11,
                ]);
            }
        }
    }
    const pixels = floatingParticles(root, ['#6cff82', '#ff5ee1', '#4edfff'], 9, 0.03);
    return {
        root,
        update: (elapsed) =>
            pixels.forEach((pixel, index) => {
                pixel.visible = Math.floor(elapsed * 5 + index) % 3 !== 0;
            }),
    };
}

function buildGlam(): BuiltLens {
    const root = new THREE.Group();
    const glasses = addFrame(root, '#ff7eb6', 0.01);
    const jewels: THREE.Mesh[] = [];
    for (const side of [-1, 1]) {
        for (let index = 0; index < 5; index += 1) {
            const angle = (index / 5) * Math.PI * 2;
            jewels.push(
                addMesh(root, new THREE.OctahedronGeometry(0.032), glass(index % 2 ? '#ffd55d' : '#ff79bf', 0.9), [
                    side * 0.235 + Math.cos(angle) * 0.2,
                    0.01 + Math.sin(angle) * 0.15,
                    0.16,
                ]),
            );
        }
        sphere(root, glass('#ffb6dd', 0.9), [side * 0.5, 0.52, 0.08], [0.55, 1.35, 0.42]);
    }
    return {
        root,
        update: (elapsed) => {
            glasses.rotation.y = Math.sin(elapsed) * 0.025;
            jewels.forEach((jewel, index) => (jewel.rotation.y = elapsed * (1 + index * 0.03)));
        },
    };
}

export function buildLens(id: LensId): BuiltLens {
    switch (id) {
        case 'cyber':
            return buildCyber();
        case 'crystal-crown':
            return buildCrown();
        case 'cat':
            return buildCat();
        case 'puppy':
            return buildJeelizPuppy();
        case 'devil':
            return buildDevil();
        case 'angel':
            return buildAngel();
        case 'space':
            return buildSpace();
        case 'party':
            return buildParty();
        case 'butterfly':
            return buildButterfly();
        case 'frog':
            return buildFrog();
        case 'robot':
            return buildRobot();
        case 'masquerade':
            return buildMasquerade();
        case 'ice':
            return buildIce();
        case 'arcade':
            return buildArcade();
        case 'glam':
            return buildGlam();
        case 'disco-outlaw':
            return buildDiscoOutlaw();
        case 'red-flag-royalty':
            return buildRedFlagRoyalty();
        case 'bad-decisions':
            return buildBadDecisions();
        default:
            return { root: new THREE.Group(), update: () => undefined };
    }
}
