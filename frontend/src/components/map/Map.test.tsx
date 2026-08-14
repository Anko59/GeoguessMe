import { createElement } from 'react';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import Map from './Map';

// react-leaflet renders a real map that needs a browser viewport; mock the
// leaflet-heavy parts so we can still exercise the component's prop-driven
// rendering and the LocationMarker logic.
function createComponent(displayName: string) {
    const Comp = (props: Record<string, unknown>) => createElement('div', { 'data-testid': displayName, ...props });
    Comp.displayName = displayName;
    return Comp;
}

vi.mock('react-leaflet', () => ({
    MapContainer: createComponent('MapContainer'),
    TileLayer: createComponent('TileLayer'),
    Marker: createComponent('Marker'),
    Popup: createComponent('Popup'),
    useMap: vi.fn(),
    useMapEvents: vi.fn(),
}));

vi.mock('leaflet', () => ({
    default: {
        icon: () => 'mock-icon',
        divIcon: () => 'mock-div-icon',
        latLngBounds: (points: unknown) => ({ points, _leafletBounds: true }),
        Marker: { prototype: { options: { icon: null } } },
    },
    icon: () => 'mock-icon',
    divIcon: () => 'mock-div-icon',
    latLngBounds: (points: unknown) => ({ points, _leafletBounds: true }),
    Marker: { prototype: { options: { icon: null } } },
}));

// Mock the CSS and image imports that vite handles.
vi.mock('leaflet/dist/leaflet.css', () => ({}));
vi.mock('leaflet/dist/images/marker-icon.png', () => ({ default: 'marker-icon.png' }));
vi.mock('leaflet/dist/images/marker-shadow.png', () => ({ default: 'marker-shadow.png' }));

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const { useMap, useMapEvents } = vi.mocked(await import('react-leaflet')) as any;

beforeEach(() => {
    vi.clearAllMocks();
    useMap.mockReturnValue({ fitBounds: vi.fn() });
});

describe('Map component', () => {
    it('renders the map container', () => {
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} />);
        expect(screen.getByRole('application', { name: 'Guess map' })).toBeInTheDocument();
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

    it('renders guess markers with a popup naming the guesser', () => {
        const guesses = [
            { user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 },
            { user_id: 'u2', lat: 49.0, long: 2.5, username: 'bob', avatar: 'b.png', score: 80 },
        ];
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} guesses={guesses} />);
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(2);
        expect(markers[0]).toHaveAttribute('position', '48.8,2.3');
        expect(markers[0]).toHaveAttribute('opacity', '0.8');
        const firstPopup = markers[0].querySelector('[data-testid="Popup"]') as HTMLElement;
        expect(firstPopup).toHaveTextContent('alice');
        expect(firstPopup).toHaveTextContent('100 pts');
        expect(markers[1]).toHaveAttribute('position', '49,2.5');
        expect(markers[1].querySelector('[data-testid="Popup"]')).toHaveTextContent('bob');
    });

    it('skips guesses without coordinates (hidden-location challenges)', () => {
        const guesses = [
            { user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 },
            // The other guesser's point is omitted by the server while the
            // challenge location is hidden; no marker may render for it.
            { user_id: 'u2', username: 'bob', avatar: 'b.png', score: 80 },
        ];
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} guesses={guesses} />);
        const markers = screen.getAllByTestId('Marker');
        expect(markers).toHaveLength(1);
        expect(markers[0]).toHaveAttribute('position', '48.8,2.3');
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

    it('fits the view to every guess and the revealed spot', () => {
        const fitBounds = vi.fn();
        useMap.mockReturnValue({ fitBounds });
        const guesses = [
            { user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 },
            { user_id: 'u2', lat: 49.0, long: 2.5, username: 'bob', avatar: 'b.png', score: 80 },
        ];
        render(
            <Map
                onLocationSelect={vi.fn()}
                selectedLocation={null}
                actualLocation={{ lat: 48.85, long: 2.4 }}
                guesses={guesses}
            />,
        );
        expect(fitBounds).toHaveBeenCalledTimes(1);
        expect(fitBounds).toHaveBeenCalledWith(
            expect.objectContaining({
                points: expect.arrayContaining([
                    [48.8, 2.3],
                    [49.0, 2.5],
                    [48.85, 2.4],
                ]),
            }),
            expect.objectContaining({ padding: [40, 40], maxZoom: 17 }),
        );
    });

    it('keeps the world view when there are no points to fit', () => {
        const fitBounds = vi.fn();
        useMap.mockReturnValue({ fitBounds });
        render(<Map onLocationSelect={vi.fn()} selectedLocation={null} />);
        expect(fitBounds).not.toHaveBeenCalled();
    });

    it('refits only when the points change, never on plain re-renders', () => {
        const fitBounds = vi.fn();
        useMap.mockReturnValue({ fitBounds });
        const guesses = [{ user_id: 'u1', lat: 48.8, long: 2.3, username: 'alice', avatar: 'a.png', score: 100 }];
        const { rerender } = render(<Map onLocationSelect={vi.fn()} selectedLocation={null} guesses={guesses} />);
        // A re-render with the same points (e.g. the results clock tick) must
        // not re-fit, otherwise it would fight the user's manual zoom.
        rerender(<Map onLocationSelect={vi.fn()} selectedLocation={null} guesses={guesses} />);
        expect(fitBounds).toHaveBeenCalledTimes(1);
        rerender(
            <Map
                onLocationSelect={vi.fn()}
                selectedLocation={null}
                guesses={[{ user_id: 'u1', lat: 48.81, long: 2.31, username: 'alice', avatar: 'a.png', score: 100 }]}
            />,
        );
        expect(fitBounds).toHaveBeenCalledTimes(2);
    });
});
