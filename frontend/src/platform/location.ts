import { Geolocation } from '@capacitor/geolocation';
import { isNativeRuntime } from './runtime';

const LOCATION_OPTIONS: PositionOptions = {
    enableHighAccuracy: false,
    timeout: 10_000,
    maximumAge: 60_000,
};

const NATIVE_LOCATION_OPTIONS: PositionOptions = {
    ...LOCATION_OPTIONS,
    // Challenge scoring needs a real GPS fix. Balanced Android location can
    // remain unresolved when only the emulator/handset GPS provider is active.
    enableHighAccuracy: true,
};

export async function getCurrentPosition(): Promise<GeolocationPosition> {
    if (isNativeRuntime()) {
        return (await Geolocation.getCurrentPosition(NATIVE_LOCATION_OPTIONS)) as unknown as GeolocationPosition;
    }
    return new Promise((resolve, reject) => {
        if (!navigator.geolocation) {
            reject(new Error('Geolocation is not supported by your browser'));
            return;
        }
        navigator.geolocation.getCurrentPosition(resolve, reject, LOCATION_OPTIONS);
    });
}
