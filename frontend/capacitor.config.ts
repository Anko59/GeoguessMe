import type { CapacitorConfig } from '@capacitor/cli';
// Capacitor intercepts every GET on its asset host, including /api/v1 paths.
// The virtual origin must never be the backend or public web origin.
const nativeAssetHostname = 'app.geoguessme.com';

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
              // Only packaged assets live here. Network API/OIDC requests go
              // to the configured backend host, which must allow this origin.
              hostname: nativeAssetHostname,
              androidScheme: 'https',
          },
    plugins: {
        CapacitorCookies: { enabled: true },
        Keyboard: { resize: 'body', resizeOnFullScreen: true },
        SystemBars: { insetsHandling: 'css' },
    },
};

export default config;
