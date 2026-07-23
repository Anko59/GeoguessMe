import type { NormalizedLandmark } from '@mediapipe/tasks-vision';

export interface FaceFrame {
    landmarks: NormalizedLandmark[];
    blendshapes: Readonly<Record<string, number>>;
}

export interface FacePose {
    centerX: number;
    centerY: number;
    faceWidth: number;
    faceHeight: number;
    roll: number;
    yaw: number;
    pitch: number;
    mouthOpen: number;
    smile: number;
    blinkLeft: number;
    blinkRight: number;
}

const clamp = (value: number, minimum: number, maximum: number): number => Math.min(maximum, Math.max(minimum, value));

const distance = (first: NormalizedLandmark, second: NormalizedLandmark, width: number, height: number): number =>
    Math.hypot((second.x - first.x) * width, (second.y - first.y) * height);

export function deriveFacePose(frame: FaceFrame, width: number, height: number): FacePose | null {
    const landmarks = frame.landmarks;
    if (landmarks.length < 468 || width <= 0 || height <= 0) return null;

    const leftCheek = landmarks[234];
    const rightCheek = landmarks[454];
    const forehead = landmarks[10];
    const chin = landmarks[152];
    const leftEye = landmarks[33];
    const rightEye = landmarks[263];
    const noseBridge = landmarks[168];
    const noseTip = landmarks[1];
    const upperLip = landmarks[13];
    const lowerLip = landmarks[14];

    const faceWidth = distance(leftCheek, rightCheek, width, height);
    const faceHeight = distance(forehead, chin, width, height);
    if (faceWidth < 12 || faceHeight < 16) return null;

    const eyeMidY = (leftEye.y + rightEye.y) / 2;
    const cheekMidX = (leftCheek.x + rightCheek.x) / 2;
    const roll = Math.atan2((rightEye.y - leftEye.y) * height, (rightEye.x - leftEye.x) * width);
    const yaw = clamp(((noseTip.x - cheekMidX) * width * 2.5) / faceWidth, -0.75, 0.75);
    const neutralNoseOffset = 0.34;
    const pitch = clamp((((noseTip.y - eyeMidY) * height) / faceHeight - neutralNoseOffset) * 2.8, -0.55, 0.55);
    const geometricMouthOpen = distance(upperLip, lowerLip, width, height) / faceHeight;
    const shape = frame.blendshapes;

    return {
        centerX: noseBridge.x * width,
        centerY: noseBridge.y * height,
        faceWidth,
        faceHeight,
        roll,
        yaw,
        pitch,
        mouthOpen: clamp(Math.max(shape.jawOpen ?? 0, geometricMouthOpen * 8), 0, 1),
        smile: clamp(Math.max(shape.mouthSmileLeft ?? 0, shape.mouthSmileRight ?? 0), 0, 1),
        blinkLeft: clamp(shape.eyeBlinkLeft ?? 0, 0, 1),
        blinkRight: clamp(shape.eyeBlinkRight ?? 0, 0, 1),
    };
}

const interpolateAngle = (from: number, to: number, amount: number): number => {
    let difference = to - from;
    if (difference > Math.PI) difference -= Math.PI * 2;
    if (difference < -Math.PI) difference += Math.PI * 2;
    return from + difference * amount;
};

export function smoothFacePose(previous: FacePose | null, next: FacePose | null): FacePose | null {
    if (!next) return null;
    if (!previous) return next;
    const amount = 0.42;
    const mix = (from: number, to: number): number => from + (to - from) * amount;
    return {
        centerX: mix(previous.centerX, next.centerX),
        centerY: mix(previous.centerY, next.centerY),
        faceWidth: mix(previous.faceWidth, next.faceWidth),
        faceHeight: mix(previous.faceHeight, next.faceHeight),
        roll: interpolateAngle(previous.roll, next.roll, amount),
        yaw: mix(previous.yaw, next.yaw),
        pitch: mix(previous.pitch, next.pitch),
        mouthOpen: mix(previous.mouthOpen, next.mouthOpen),
        smile: mix(previous.smile, next.smile),
        blinkLeft: mix(previous.blinkLeft, next.blinkLeft),
        blinkRight: mix(previous.blinkRight, next.blinkRight),
    };
}
