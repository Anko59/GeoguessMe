import { act, render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLensEffects } from './useLensEffects';

interface FakeRenderer {
    canvas: HTMLCanvasElement;
    setSource: ReturnType<typeof vi.fn>;
    resize: ReturnType<typeof vi.fn>;
    setLens: ReturnType<typeof vi.fn>;
    render: ReturnType<typeof vi.fn>;
    clear: ReturnType<typeof vi.fn>;
    dispose: ReturnType<typeof vi.fn>;
}

interface FakeTracker {
    close: ReturnType<typeof vi.fn>;
    detectVideo: ReturnType<typeof vi.fn>;
    detectImage: ReturnType<typeof vi.fn>;
}

const rendererState = vi.hoisted(() => ({ instances: [] as FakeRenderer[] }));
const trackerState = vi.hoisted(() => ({
    instances: [] as FakeTracker[],
    failNextCreate: false,
    makeTracker: null as null | (() => unknown),
    factoryMakeTracker: null as null | (() => FakeTracker),
}));

vi.mock('../lenses/LensRenderer', () => {
    class FakeRenderer {
        canvas: HTMLCanvasElement;
        setSource = vi.fn();
        resize = vi.fn();
        setLens = vi.fn();
        render = vi.fn();
        clear = vi.fn();
        dispose = vi.fn();

        constructor(canvas: HTMLCanvasElement) {
            this.canvas = canvas;
            rendererState.instances.push(this as unknown as FakeRenderer);
        }
    }
    return { LensRenderer: FakeRenderer };
});

vi.mock('../lenses/faceTracker', () => {
    class FakeTracker {
        close = vi.fn();
        detectVideo = vi.fn(() => ({ landmarks: [], blendshapes: {} }));
        detectImage = vi.fn(async () => ({ landmarks: [], blendshapes: {} }));

        constructor() {
            trackerState.instances.push(this as unknown as FakeTracker);
        }

        static async create(): Promise<unknown> {
            if (trackerState.failNextCreate) {
                trackerState.failNextCreate = false;
                throw new Error('tracker failed');
            }
            return trackerState.makeTracker?.();
        }
    }
    trackerState.factoryMakeTracker = () => new FakeTracker() as unknown as FakeTracker;
    trackerState.makeTracker = trackerState.factoryMakeTracker;
    return { FaceTracker: FakeTracker };
});

const videoSource = { readyState: 2, currentTime: 0 } as unknown as HTMLVideoElement;
const imageSource = { width: 640, height: 480 } as unknown as HTMLCanvasElement;

type LensApi = ReturnType<typeof useLensEffects>;

function renderHarness() {
    const api: { current: LensApi | null } = { current: null };
    function Harness() {
        const lens = useLensEffects();
        api.current = lens;
        return <canvas ref={lens.overlayCanvasRef} />;
    }
    const view = render(<Harness />);
    return { api, unmount: view.unmount };
}

function deferredTracker() {
    let resolveTracker: ((tracker: unknown) => void) | null = null;
    const blocked = new Promise<unknown>((resolve) => {
        resolveTracker = resolve;
    });
    const originalMake = trackerState.makeTracker;
    trackerState.makeTracker = () => blocked;
    return {
        blocked,
        resolveTracker: resolveTracker as ((tracker: unknown) => void) | null,
        originalMake,
        restore: () => {
            trackerState.makeTracker = originalMake;
        },
    };
}

beforeEach(() => {
    vi.clearAllMocks();
    rendererState.instances.length = 0;
    trackerState.instances.length = 0;
    trackerState.failNextCreate = false;
    trackerState.makeTracker = trackerState.factoryMakeTracker;
    vi.stubGlobal(
        'requestAnimationFrame',
        vi.fn(() => 1),
    );
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
});

afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});

describe('useLensEffects', () => {
    it('creates the renderer and tracker and starts the tracking loop for a video', async () => {
        const { api } = renderHarness();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480);
        });

        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(1);
            expect(trackerState.instances).toHaveLength(1);
            expect(api.current!.filterReady).toBe(true);
        });
        expect(requestAnimationFrame).toHaveBeenCalled();
        expect(rendererState.instances[0].setLens).toHaveBeenCalledWith('none');
    });

    it('destroys effects by cancelling the frame and disposing the renderer and tracker', async () => {
        const { api } = renderHarness();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480);
        });
        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(1);
            expect(trackerState.instances).toHaveLength(1);
        });

        act(() => {
            api.current!.destroyEffects();
        });
        expect(cancelAnimationFrame).toHaveBeenCalledWith(1);
        expect(rendererState.instances[0].dispose).toHaveBeenCalled();
        expect(trackerState.instances[0].close).toHaveBeenCalled();
        expect(api.current!.filterReady).toBe(false);
    });

    it('closes a tracker that resolves after effects are destroyed', async () => {
        const { api } = renderHarness();
        const deferred = deferredTracker();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480); // in-flight, blocked at tracker
        });
        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(1);
            expect(trackerState.instances).toHaveLength(0);
        });

        act(() => {
            api.current!.destroyEffects(); // supersedes the in-flight attempt
        });
        expect(rendererState.instances[0].dispose).toHaveBeenCalled();
        expect(api.current!.rendererRef.current).toBeNull();

        await act(async () => {
            deferred.resolveTracker!(deferred.originalMake!());
            await Promise.resolve();
        });
        // The late tracker observed the new generation and was closed, never attached.
        expect(trackerState.instances).toHaveLength(1);
        expect(trackerState.instances[0].close).toHaveBeenCalled();
        expect(api.current!.trackerRef.current).toBeNull();
        expect(api.current!.filterReady).toBe(false);
    });

    it('supersedes an in-flight attempt: disposes the replaced renderer and closes the late tracker', async () => {
        const { api } = renderHarness();
        const deferred = deferredTracker();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480); // attempt 1, blocked at tracker
        });
        await waitFor(() => expect(rendererState.instances).toHaveLength(1));

        deferred.restore();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480); // attempt 2 supersedes
        });
        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(2);
            expect(trackerState.instances).toHaveLength(1);
        });
        // Attempt 1's renderer was replaced and disposed; attempt 2's renderer is live.
        expect(rendererState.instances[0].dispose).toHaveBeenCalled();
        expect(rendererState.instances[1].dispose).not.toHaveBeenCalled();
        expect(api.current!.rendererRef.current).toBe(rendererState.instances[1]);

        await act(async () => {
            deferred.resolveTracker!(deferred.originalMake!());
            await Promise.resolve();
        });
        // The late tracker is closed, not attached; attempt 2's tracker stays live.
        expect(trackerState.instances).toHaveLength(2);
        expect(trackerState.instances[1].close).toHaveBeenCalled();
        expect(trackerState.instances[0].close).not.toHaveBeenCalled();
        expect(api.current!.trackerRef.current).toBe(trackerState.instances[0]);
    });

    it('reports a tracker failure and disposes the renderer', async () => {
        const { api } = renderHarness();
        trackerState.failNextCreate = true;
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480);
        });

        await waitFor(() => {
            expect(api.current!.filterError).toBe(
                'Face tracking could not start. Photos can still be sent without a lens.',
            );
        });
        expect(rendererState.instances[0].dispose).toHaveBeenCalled();
        expect(api.current!.rendererRef.current).toBeNull();
        expect(api.current!.filterReady).toBe(false);
    });

    it('initializes image effects by rendering the detected frame', async () => {
        const { api } = renderHarness();
        act(() => {
            void api.current!.initializeImageEffects(imageSource, 640, 480);
        });

        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(1);
            expect(trackerState.instances).toHaveLength(1);
            expect(api.current!.filterReady).toBe(true);
        });
        expect(trackerState.instances[0].detectImage).toHaveBeenCalledWith(imageSource);
        expect(rendererState.instances[0].render).toHaveBeenCalled();
    });

    it('disposes all lens resources on unmount', async () => {
        const { api, unmount } = renderHarness();
        act(() => {
            void api.current!.initializeVideoEffects(videoSource, 640, 480);
        });
        await waitFor(() => {
            expect(rendererState.instances).toHaveLength(1);
            expect(trackerState.instances).toHaveLength(1);
        });

        unmount();
        expect(rendererState.instances[0].dispose).toHaveBeenCalled();
        expect(trackerState.instances[0].close).toHaveBeenCalled();
        expect(cancelAnimationFrame).toHaveBeenCalledWith(1);
    });
});
