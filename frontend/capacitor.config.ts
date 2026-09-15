import type { CapacitorConfig } from '@capacitor/cli';
import { productionHostname } from './src/platform/production';

const serverURL = process.env.CAPACITOR_SERVER_URL?.trim();

const config: CapacitorConfig = {
    appId: 'com.geoguessme.app',
    appName: 'GeoGuessMe',
    webDir: 'dist',
    loggingBehavior: 'none',
    android: {
        path: 'android',
        minWebViewVersion: 105,
    },
    server: serverURL
        ? {
              url: serverURL,
              cleartext: serverURL.startsWith('http://'),
          }
        : {
              // Keep the native WebView origin on the same production host as
              // the bundled API and public web URLs. This origin is used for
              // cookie and CORS decisions even though the web assets are
              // packaged inside the APK.
              hostname: productionHostname,
              androidScheme: 'https',
          },
    plugins: {
        CapacitorCookies: { enabled: true },
        Keyboard: { resize: 'body', resizeOnFullScreen: true },
        SystemBars: { insetsHandling: 'css' },
    },
};

export default config;
