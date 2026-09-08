import { Share } from '@capacitor/share';
import { isNativeRuntime } from './runtime';

export type ShareResult = 'shared' | 'copied' | 'unavailable';

export async function shareOrCopy(title: string, text: string, url: string): Promise<ShareResult> {
    if (isNativeRuntime()) {
        try {
            await Share.share({ title, text, url, dialogTitle: title });
            return 'shared';
        } catch {
            // A dismissed native share sheet is not an error worth showing;
            // fall through to the clipboard only when it is available.
        }
    }
    if (!navigator.clipboard) return 'unavailable';
    try {
        await navigator.clipboard.writeText(url);
        return 'copied';
    } catch {
        return 'unavailable';
    }
}
