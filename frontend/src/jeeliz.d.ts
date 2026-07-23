interface JeelizFaceFilterInitResult {
    canvasElement: HTMLCanvasElement;
}

interface JeelizFaceFilterApi {
    init(parameters: {
        canvas: HTMLCanvasElement;
        NNCPath: string;
        videoSettings: { videoElement: HTMLVideoElement };
        callbackReady: (errorCode: string | false, result: JeelizFaceFilterInitResult) => void;
        callbackTrack: (state: {
            detected: number;
            x: number;
            y: number;
            s: number;
            rx: number;
            ry: number;
            rz: number;
            expressions: Float32Array;
        }) => void;
    }): boolean;
    render_video(): void;
    destroy(): Promise<void>;
}

interface Window {
    JEELIZFACEFILTER?: JeelizFaceFilterApi;
}
