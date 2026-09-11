import { useId, useState } from 'react';
import Map from '../../components/map/Map';

function parsePoint(latitude: string, longitude: string) {
    const lat = Number(latitude);
    const long = Number(longitude);
    if (
        latitude === '' ||
        longitude === '' ||
        !Number.isFinite(lat) ||
        !Number.isFinite(long) ||
        Math.abs(lat) > 90 ||
        Math.abs(long) > 180
    )
        return null;
    return { lat, long };
}

export default function LocationPicker({
    onChange,
}: {
    onChange: (point: { lat: number; long: number } | null) => void;
}) {
    const id = useId();
    const [lat, setLat] = useState('');
    const [long, setLong] = useState('');
    const selected = parsePoint(lat, long);
    function change(latitude: string, longitude: string) {
        setLat(latitude);
        setLong(longitude);
        onChange(parsePoint(latitude, longitude));
    }
    return (
        <div className="feed-location-picker">
            <p>Choose a point on the map, or enter coordinates.</p>
            <div className="feed-map">
                <Map
                    selectedLocation={selected}
                    onLocationSelect={(latitude, longitude) =>
                        change(latitude.toFixed(6), (((((longitude + 180) % 360) + 360) % 360) - 180).toFixed(6))
                    }
                />
            </div>
            <div className="feed-coordinates">
                <label htmlFor={`${id}-lat`}>
                    Latitude
                    <input
                        id={`${id}-lat`}
                        type="number"
                        step="any"
                        min="-90"
                        max="90"
                        required
                        value={lat}
                        onChange={(e) => change(e.target.value, long)}
                    />
                </label>
                <label htmlFor={`${id}-long`}>
                    Longitude
                    <input
                        id={`${id}-long`}
                        type="number"
                        step="any"
                        min="-180"
                        max="180"
                        required
                        value={long}
                        onChange={(e) => change(lat, e.target.value)}
                    />
                </label>
            </div>
        </div>
    );
}
