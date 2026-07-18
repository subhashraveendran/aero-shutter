import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.aeroshutter.app',
  appName: 'AeroShutter',
  webDir: 'dist',
  backgroundColor: '#0a0a0b',
  plugins: {
    SplashScreen: {
      launchShowDuration: 900,
      backgroundColor: '#0a0a0b',
      showSpinner: false,
      androidSplashResourceName: 'splash',
    },
    StatusBar: {
      style: 'DARK',
      backgroundColor: '#0a0a0b',
      overlaysWebView: false,
    },
  },
  ios: {
    contentInset: 'never',
  },
  server: {
    androidScheme: 'https',
  },
};

export default config;
