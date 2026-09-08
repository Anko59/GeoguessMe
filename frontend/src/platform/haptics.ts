import { Haptics, ImpactStyle, NotificationType } from '@capacitor/haptics';
import { isNativeRuntime } from './runtime';

export async function captureFeedback(): Promise<void> {
    if (!isNativeRuntime()) return;
    await Haptics.impact({ style: ImpactStyle.Light }).catch(() => undefined);
}

export async function successFeedback(): Promise<void> {
    if (!isNativeRuntime()) return;
    await Haptics.notification({ type: NotificationType.Success }).catch(() => undefined);
}
