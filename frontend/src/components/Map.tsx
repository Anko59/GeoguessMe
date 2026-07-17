import { MapContainer, TileLayer, Marker, useMapEvents } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import L from 'leaflet';

// Fix for default marker icon
import icon from 'leaflet/dist/images/marker-icon.png';
import iconShadow from 'leaflet/dist/images/marker-shadow.png';

const DefaultIcon = L.icon({
    iconUrl: icon,
    shadowUrl: iconShadow,
    iconSize: [25, 41],
    iconAnchor: [12, 41]
});

L.Marker.prototype.options.icon = DefaultIcon;

interface Guess {
    user_id: string;
    lat: number;
    long: number;
    username: string;
    avatar: string;
    score: number;
}

interface MapProps {
    onLocationSelect: (lat: number, long: number) => void;
    selectedLocation: { lat: number; long: number } | null;
    actualLocation?: { lat: number; long: number } | null;
    guesses?: Guess[];
}

function LocationMarker({ onLocationSelect, position }: { onLocationSelect: (lat: number, long: number) => void, position: { lat: number; long: number } | null }) {
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
    iconAnchor: [8, 8]
});

export default function Map({ onLocationSelect, selectedLocation, actualLocation, guesses }: MapProps) {
    return (
        <MapContainer center={[20, 0]} zoom={2} style={{ height: '100%', width: '100%' }}>
            <TileLayer
                attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
                url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
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
                        shadowSize: [41, 41]
                    })}
                />
            )}

            {/* User Guesses */}
            {guesses?.map((guess, idx) => (
                <Marker
                    key={idx}
                    position={[guess.lat, guess.long]}
                    icon={GuessIcon}
                    title={`${guess.username}: ${guess.score} pts`}
                    opacity={0.8}
                />
            ))}
        </MapContainer>
    );
}
