import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Map from './Map';

// react-leaflet renders a real map that needs a browser viewport; mock the
// leaflet-heavy parts so we can still exercise the component's prop-driven
// rendering and the LocationMarker logic.
function createComponent(displayName: string) {
    const Comp = (props: Record<string, unknown>) =>
        // Use a simple wrapper to avoid importing React just for createElement.
        // eslint-disable-next-line @typescript-eslint/no-require-imports
        require('react').createElement('div', { 'data-testid': displayName, ...props });
    Comp.displayName = displayName;
    return Comp;
}

vi.mock('react-leaflet', () => ({
    MapContainer: createComponent('MapContainer'),
    TileLayer: createComponent('TileLayer'),
    Marker: createComponent('Marker'),
    useMapEvents: vi.fn(),
}));

vi.mock('leaflet', () => ({
    default: {
        icon: () => 'mock-icon',
        divIcon: () => 'mock-div-icon',
        Marker: { prototype: { options: { icon: null } } },
    },
    icon: () => 'mock-icon',
    divIcon: () => 'mock-div-icon',
    Marker: { prototype: { options: { icon: null } } },
}));

// Mock the CSS and image imports that vite handles.
vi.mock('leaflet/dist/leaflet.css', () => ({}));
vi.mock('leaflet/dist/images/marker-icon.png', () => ({ default: 'marker-icon.png' }));
vi.mock('leaflet/dist/images/marker-shadow.png', () => ({ default: 'marker-shadow.png' }));

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const { useMapEvents } = vi.mocked(await import('react-leaflet')) as any;

beforeEach(() => {
    vi.clearAllMocks();
});

describe('Map component', () => {
    it('renders the map container', () => {
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} />);
        expect(screen.getByTestId('MapContainer')).toBeInTheDocument();
        expect(screen.getByTestId('TileLayer')).toBeInTheDocument();
    });

    it('registers a map click handler via useMapEvents', () => {
        const onLocationSelect = vi.fn();
        render(<Map onLocationSelect={onLocationSelect} selectedLocation={null} />);
        expect(useMapEvents).toHaveBeenCalled();
        const config = useMapEvents.mock.calls[0]?.[0];
        expect(config).toBeDefined();
        expect(typeof config.click).toBe('function');
        config.click({ latlng: { lat: 48.8, lng: 2.3 } });
        expect(onLocationSelect).toHaveBeenCalledWith(48.8, 2.3);
    });

    it('renders a Marker when a selected location is provided', () => {
        render(<Map onLocationSelect={vi.fn()} selectedLocation={{ lat: 51.5, long: -0.1 }} />);
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(1);
        expect(markers[0]).toHaveAttribute('position', '51.5,-0.1');
    });

    it('renders no LocationMarker when selectedLocation is null', () => {
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} />);
        expect(screen.queryByTestId('Marker')).not.toBeInTheDocument();
    });

    it('renders an actual location marker when provided', () => {
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} actualLocation={{ lat: 40.7, long: -74.0 }} />);
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(1);
        expect(markers[0]).toHaveAttribute('position', '40.7,-74');
    });

    it('renders guess markers for every provided guess', () => {
        const guesses = [
            { user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 },
            { user_id: 'u2', lat: 49.0, long: 2.5, username: 'bob', avatar: 'b.png', score: 80 },
        ];
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} guesses={guesses} />);
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(2);
        expect(markers[0]).toHaveAttribute('position', '48.8,2.3');
        expect(markers[0]).toHaveAttribute('title', 'alice: 100 pts');
        expect(markers[0]).toHaveAttribute('opacity', '0.8');
        expect(markers[1]).toHaveAttribute('position', '49,2.5');
        expect(markers[1]).toHaveAttribute('title', 'bob: 80 pts');
    });

    it('renders selected, actual, and guess markers together', () => {
        const guesses = [{ user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 }];
        render(
            <Map
                onLocationSelect={vi.fn()}
                selectedLocation={{ lat: 48.9, long: 2.4 }}
                actualLocation={{ lat: 48.8, long: 2.3 }}
                guesses={guesses}
            />,
        );
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(3);
    });
});
