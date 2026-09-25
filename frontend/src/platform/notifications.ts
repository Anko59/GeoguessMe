import { isNativeRuntime } from './runtime';

/** Web Push remains the browser/PWA transport. Native push needs an FCM/APNs
 * registration contract and is deliberately not impersonated as Web Push. */
export function shouldUseWebPush(): boolean {
    return !isNativeRuntime();
}
