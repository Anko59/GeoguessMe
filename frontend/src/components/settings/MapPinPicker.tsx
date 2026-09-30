import { useCallback, useEffect, useRef, useState } from 'react';
import api, { getAPIErrorMessage } from '../../api';
import type { MapPinCatalog } from '../../types';
import { DEFAULT_MAP_PIN_IMAGE_URL } from '../../utils/mapPins';
import './MapPinPicker.css';

export default function MapPinPicker() {
    const [catalog, setCatalog] = useState<MapPinCatalog | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const active = useRef(true);

    const loadCatalog = useCallback(async (isActive: () => boolean = () => active.current) => {
        try {
            const response = await api.post<MapPinCatalog>('/auth/pins');
            if (isActive()) setCatalog(response.data);
        } catch (requestError: unknown) {
            if (isActive()) setError(getAPIErrorMessage(requestError, 'Unable to load map pins.'));
        } finally {
            if (isActive()) setLoading(false);
        }
    }, []);

    useEffect(() => {
        let current = true;
        active.current = true;
        void Promise.resolve().then(() => loadCatalog(() => current));
        return () => {
            current = false;
            active.current = false;
        };
    }, [loadCatalog]);

    const selectPin = async (pinKey: string | null): Promise<void> => {
        setSaving(true);
        setError('');
        try {
            if (pinKey === null) {
                await api.delete('/auth/pins');
                if (active.current) {
                    setCatalog((current) => (current ? { ...current, selected_pin_key: null } : current));
                }
            } else {
                const response = await api.put<MapPinCatalog>('/auth/pins', { pin_key: pinKey });
                if (active.current) setCatalog(response.data);
            }
        } catch (requestError: unknown) {
            if (active.current) setError(getAPIErrorMessage(requestError, 'Unable to update your map pin.'));
        } finally {
            if (active.current) setSaving(false);
        }
    };

    return (
        <section className="map-pin-picker" aria-labelledby="map-pin-picker-title" aria-busy={loading || saving}>
            <div className="map-pin-picker__heading">
                <h3 id="map-pin-picker-title">Map pin</h3>
                <p>Choose the marker shown for you on challenge maps and the group globe.</p>
            </div>
            {loading ? (
                <p role="status">Loading map pins…</p>
            ) : error && !catalog ? (
                <div className="map-pin-picker__error" role="alert">
                    <span>{error}</span>
                    <button
                        className="btn btn-secondary"
                        type="button"
                        onClick={() => {
                            setLoading(true);
                            setError('');
                            void loadCatalog();
                        }}
                    >
                        Retry
                    </button>
                </div>
            ) : catalog ? (
                <>
                    {error && (
                        <p className="map-pin-picker__error" role="alert">
                            {error}
                        </p>
                    )}
                    <div className="map-pin-picker__choices" role="group" aria-label="Available map pins" tabIndex={0}>
                        <article className="map-pin-choice">
                            <img className="map-pin-choice__default" src={DEFAULT_MAP_PIN_IMAGE_URL} alt="" />
                            <div className="map-pin-choice__copy">
                                <strong>Standard marker</strong>
                                <span>Use the default marker.</span>
                            </div>
                            <button
                                className="btn btn-secondary"
                                type="button"
                                aria-pressed={catalog.selected_pin_key === null}
                                disabled={saving || catalog.selected_pin_key === null}
                                onClick={() => void selectPin(null)}
                            >
                                {catalog.selected_pin_key === null ? 'Selected' : 'Use this'}
                            </button>
                        </article>
                        {catalog.pins.map((pin) => (
                            <article className={`map-pin-choice${pin.unlocked ? '' : ' is-locked'}`} key={pin.key}>
                                <img className="map-pin-choice__image" src={pin.image_url} alt="" />
                                <div className="map-pin-choice__copy">
                                    <strong>{pin.name}</strong>
                                    <span>{pin.description}</span>
                                    {pin.challenges.map((challenge) => (
                                        <span className="map-pin-choice__challenge" key={challenge.key}>
                                            {challenge.unlocked_at ? 'Completed' : 'Unlock challenge'}: {challenge.name}
                                            {challenge.unlocked_at && (
                                                <> · {new Date(challenge.unlocked_at).toLocaleDateString()}</>
                                            )}
                                            <small>{challenge.description}</small>
                                        </span>
                                    ))}
                                </div>
                                {pin.unlocked ? (
                                    <button
                                        className="btn btn-secondary"
                                        type="button"
                                        aria-pressed={catalog.selected_pin_key === pin.key}
                                        disabled={saving || catalog.selected_pin_key === pin.key}
                                        onClick={() => void selectPin(pin.key)}
                                    >
                                        {catalog.selected_pin_key === pin.key ? 'Selected' : 'Use this'}
                                    </button>
                                ) : (
                                    <span className="map-pin-choice__locked">Locked</span>
                                )}
                            </article>
                        ))}
                    </div>
                    {catalog.pins.length === 0 && (
                        <p className="map-pin-picker__empty">No map pins are available yet.</p>
                    )}
                </>
            ) : null}
        </section>
    );
}
