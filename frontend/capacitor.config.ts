import type { CapacitorConfig } from '@capacitor/cli';

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
              hostname: 'app.geoguessme.com',
              androidScheme: 'https',
          },
    plugins: {
        CapacitorCookies: { enabled: true },
        Keyboard: { resize: 'body', resizeOnFullScreen: true },
        SystemBars: { insetsHandling: 'css' },
    },
};

export default config;
