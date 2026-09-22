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
    const latestOnSelect = useRef(onSelect);
    const latestView = useRef({ items, selectedID });
    const [error, setError] = useState('');
    const [detailUnavailable, setDetailUnavailable] = useState(false);
    const [detailSources, setDetailSources] = useState<('nasa' | 'osm')[]>([]);
    const [textureReady, setTextureReady] = useState(false);

    useEffect(() => {
        latestOnSelect.current = onSelect;
    }, [onSelect]);

    useEffect(() => {
        latestView.current = { items, selectedID };
        scene.current?.update(items, selectedID);
    }, [items, selectedID]);

    useEffect(() => {
        let active = true;
        void import('./globeScene')
            .then(({ createGlobeScene }) => {
                if (!active || !host.current) return;
                const nextScene = createGlobeScene(
                    host.current,
                    (id) => latestOnSelect.current(id),
                    (message) => {
                        if (active) setError(message);
                    },
                    () => {
                        if (active) setTextureReady(true);
                    },
                    (unavailable) => {
                        if (active) setDetailUnavailable(unavailable);
                    },
                    (providers) => {
                        if (active) setDetailSources(providers);
                    },
                );
                if (!active) {
                    nextScene.dispose();
                    return;
                }
                scene.current = nextScene;
                const { items: currentItems, selectedID: currentSelectedID } = latestView.current;
                nextScene.update(currentItems, currentSelectedID);
                const selected = currentItems.find((item) => item.photo_id === currentSelectedID);
                if (selected) nextScene.focus(selected);
            })
            .catch(() => {
                if (active) setError('3D rendering is unavailable. You can still browse every challenge below.');
            });
        return () => {
            active = false;
            scene.current?.dispose();
            scene.current = null;
        };
    }, []);
    const selected = items.find((item) => item.photo_id === selectedID);
    useEffect(() => {
        if (selected) scene.current?.focus(selected);
    }, [selected]);
    return (
        <div className="globe-stage">
            <div ref={host} className="globe-canvas" />
            {!textureReady && !error && (
                <p className="globe-notice" role="status">
                    Loading Earth…
                </p>
            )}
            {error && (
                <p className="globe-notice" role="status">
                    {error}
                </p>
            )}
            {detailUnavailable && textureReady && !error && (
                <p className="globe-notice globe-detail-notice" role="status">
                    Some detail imagery is unavailable; showing available layers.
                </p>
            )}
            {textureReady && !error && (
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
            <span className="globe-credit" aria-label="Earth imagery credits">
                Earth imagery:{' '}
                <a href="https://svs.gsfc.nasa.gov/3615/" target="_blank" rel="noreferrer">
                    Blue Marble Next Generation (2004)
                </a>{' '}
                · NASA/Goddard Space Flight Center
                {detailSources.includes('nasa') && (
                    <>
                        {' '}
                        · Detail imagery:{' '}
                        <a href="https://earthdata.nasa.gov/gibs" target="_blank" rel="noreferrer">
                            NASA GIBS (NASA ESDIS)
                        </a>
                    </>
                )}
                {detailSources.includes('osm') && (
                    <>
                        {' '}
                        · Map data{' '}
                        <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noreferrer">
                            © OpenStreetMap contributors
                        </a>
                    </>
                )}{' '}
                · Three.js
            </span>
        </div>
    );
}
