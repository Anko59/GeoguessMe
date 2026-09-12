import { useEffect, useRef, useState } from 'react';
import type { GroupChallenge } from '../../types';
import type { createGlobeScene } from './globeScene';

interface GlobeProps {
    items: GroupChallenge[];
    selectedID: string | null;
    onSelect: (id: string) => void;
}

export default function Globe({ items, selectedID, onSelect }: GlobeProps) {
    const host = useRef<HTMLDivElement>(null);
    const scene = useRef<ReturnType<typeof createGlobeScene> | null>(null);
    const [error, setError] = useState('');
    const [ready, setReady] = useState(false);
    useEffect(() => {
        let active = true;
        void import('./globeScene')
            .then(({ createGlobeScene }) => {
                if (!active || !host.current) return;
                scene.current = createGlobeScene(host.current, onSelect, (message) => {
                    if (active) setError(message);
                });
                setReady(true);
            })
            .catch(() => {
                if (active) setError('3D rendering is unavailable. You can still browse every challenge below.');
            });
        return () => {
            active = false;
            scene.current?.dispose();
            scene.current = null;
        };
    }, [onSelect]);
    useEffect(() => {
        scene.current?.update(items, selectedID);
    }, [items, selectedID, ready]);
    const selected = items.find((item) => item.photo_id === selectedID);
    useEffect(() => {
        if (selected) scene.current?.focus(selected);
    }, [selected, ready]);
    return (
        <div className="globe-stage">
            <div ref={host} className="globe-canvas" />
            {!ready && !error && (
                <p className="globe-notice" role="status">
                    Loading Earth…
                </p>
            )}
            {error && (
                <p className="globe-notice" role="status">
                    {error}
                </p>
            )}
            {ready && !error && (
                <div className="globe-controls" role="group" aria-label="Globe controls">
                    <button
                        type="button"
                        aria-label="Rotate globe left"
                        onClick={() => scene.current?.rotate(-0.25, 0)}
                    >
                        ←
                    </button>
                    <button
                        type="button"
                        aria-label="Rotate globe right"
                        onClick={() => scene.current?.rotate(0.25, 0)}
                    >
                        →
                    </button>
                    <button type="button" aria-label="Rotate globe up" onClick={() => scene.current?.rotate(0, -0.25)}>
                        ↑
                    </button>
                    <button type="button" aria-label="Rotate globe down" onClick={() => scene.current?.rotate(0, 0.25)}>
                        ↓
                    </button>
                    <button type="button" aria-label="Zoom in" onClick={() => scene.current?.zoom(0.8)}>
                        +
                    </button>
                    <button type="button" aria-label="Zoom out" onClick={() => scene.current?.zoom(1.25)}>
                        −
                    </button>
                </div>
            )}
            <span className="globe-credit">Earth imagery: NASA / Three.js</span>
        </div>
    );
}
