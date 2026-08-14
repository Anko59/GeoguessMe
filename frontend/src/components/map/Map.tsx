import { useEffect, useMemo } from 'react';
import { MapContainer, TileLayer, Marker, Popup, useMap, useMapEvents } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import L from 'leaflet';
import './Map.css';

// Fix for default marker icon
import icon from 'leaflet/dist/images/marker-icon.png';
import iconShadow from 'leaflet/dist/images/marker-shadow.png';

const DefaultIcon = L.icon({
    iconUrl: icon,
    shadowUrl: iconShadow,
    iconSize: [25, 41],
    iconAnchor: [12, 41],
});

L.Marker.prototype.options.icon = DefaultIcon;

interface Guess {
    user_id: string;
    lat?: number;
    long?: number;
    username: string;
    avatar: string;
    score: number;
}

function hasCoordinates(guess: Guess): guess is Guess & { lat: number; long: number } {
    return guess.lat !== undefined && guess.long !== undefined;
}

interface MapProps {
    onLocationSelect: (lat: number, long: number) => void;
    selectedLocation: { lat: number; long: number } | null;
    actualLocation?: { lat: number; long: number } | null;
    guesses?: Guess[];
}

function LocationMarker({
    onLocationSelect,
    position,
}: {
    onLocationSelect: (lat: number, long: number) => void;
    position: { lat: number; long: number } | null;
}) {
    useMapEvents({
        click(e: L.LeafletMouseEvent) {
            onLocationSelect(e.latlng.lat, e.latlng.lng);
        },
    });

    return position ? <Marker position={[position.lat, position.long]} /> : null;
}

const GuessIcon = L.divIcon({
    className: 'guess-marker',
    html: `<div style="background-color: #f59e0b; width: 12px; height: 12px; border-radius: 50%; border: 2px solid white; box-shadow: 0 2px 4px rgba(0,0,0,0.3);"></div>`,
    iconSize: [16, 16],
    iconAnchor: [8, 8],
});

/** Fits the map so every guess and the revealed spot stay visible with a
 *  little padding: close markers zoom in hard, far-apart markers zoom out.
 *  The fit runs once per set of points, so it never fights manual zooming
 *  while the results view re-renders (the 200ms clock tick re-renders the
 *  parent without changing the points). */
function FitBoundsToMarkers({
    guesses,
    actualLocation,
}: {
    guesses?: Guess[];
    actualLocation?: { lat: number; long: number } | null;
}) {
    const map = useMap();
    const actualLat = actualLocation?.lat;
    const actualLong = actualLocation?.long;
    const points = useMemo<L.LatLngTuple[]>(() => {
        const markers: L.LatLngTuple[] = [];
        if (actualLat !== undefined && actualLong !== undefined) markers.push([actualLat, actualLong]);
        for (const guess of guesses ?? []) {
            if (guess.lat !== undefined && guess.long !== undefined) markers.push([guess.lat, guess.long]);
        }
        return markers;
    }, [actualLat, actualLong, guesses]);

    useEffect(() => {
        if (points.length === 0) return;
        map.fitBounds(L.latLngBounds(points), { padding: [40, 40], maxZoom: 17 });
    }, [map, points]);

    return null;
}

export default function Map({ onLocationSelect, selectedLocation, actualLocation, guesses }: MapProps) {
    return (
        <div role="application" aria-label="Guess map" style={{ height: '100%', width: '100%' }}>
            <MapContainer center={[20, 0]} zoom={2} style={{ height: '100%', width: '100%' }}>
                <TileLayer
                    attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
                    url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
                />
                <FitBoundsToMarkers guesses={guesses} actualLocation={actualLocation} />
                <LocationMarker onLocationSelect={onLocationSelect} position={selectedLocation} />

                {/* Actual Location (Flag/Green Marker) */}
                {actualLocation && (
                    <Marker
                        position={[actualLocation.lat, actualLocation.long]}
                        opacity={1}
                        icon={L.icon({
                            iconUrl: icon,
                            shadowUrl: iconShadow,
                            iconSize: [25, 41],
                            iconAnchor: [12, 41],
                            popupAnchor: [1, -34],
                            shadowSize: [41, 41],
                        })}
                    />
                )}

                {/* User Guesses (only guesses with returned coordinates render; a
                    hidden-location challenge sends just the viewer's own point) */}
                {guesses?.filter(hasCoordinates).map((guess) => (
                    <Marker key={guess.user_id} position={[guess.lat, guess.long]} icon={GuessIcon} opacity={0.8}>
                        <Popup>
                            <strong>{guess.username}</strong>
                            <span className="guess-popup-score">{guess.score} pts</span>
                        </Popup>
                    </Marker>
                ))}
            </MapContainer>
        </div>
    );
}
