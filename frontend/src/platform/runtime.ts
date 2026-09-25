import { Capacitor } from '@capacitor/core';

export type AppRuntime = 'web' | 'android' | 'ios';

export function currentRuntime(): AppRuntime {
    const platform = Capacitor.getPlatform();
    return platform === 'android' || platform === 'ios' ? platform : 'web';
}

export function isNativeRuntime(): boolean {
    return currentRuntime() !== 'web';
}
