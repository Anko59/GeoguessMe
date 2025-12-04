import { MapContainer, TileLayer, Marker, useMapEvents } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import L from 'leaflet';

// Fix for default marker icon
import icon from 'leaflet/dist/images/marker-icon.png';
import iconShadow from 'leaflet/dist/images/marker-shadow.png';

let DefaultIcon = L.icon({
    iconUrl: icon,
    shadowUrl: iconShadow,
    iconSize: [25, 41],
    iconAnchor: [12, 41]
});

L.Marker.prototype.options.icon = DefaultIcon;

interface MapProps {
    onLocationSelect: (lat: number, long: number) => void;
    selectedLocation: { lat: number; long: number } | null;
    actualLocation?: { lat: number; long: number } | null;
}

function LocationMarker({ onLocationSelect, position }: { onLocationSelect: (lat: number, long: number) => void, position: { lat: number; long: number } | null }) {
    useMapEvents({
        click(e) {
            onLocationSelect(e.latlng.lat, e.latlng.lng);
        },
    });

    return position ? <Marker position={[position.lat, position.long]} /> : null;
}

export default function Map({ onLocationSelect, selectedLocation, actualLocation }: MapProps) {
    return (
        <MapContainer center={[20, 0]} zoom={2} style={{ height: '100%', width: '100%' }}>
            <TileLayer
                attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
                url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
            <LocationMarker onLocationSelect={onLocationSelect} position={selectedLocation} />
            {actualLocation && <Marker position={[actualLocation.lat, actualLocation.long]} opacity={0.6} />}
        </MapContainer>
    );
}
