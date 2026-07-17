import type { CapacitorConfig } from '@capacitor/cli';

const config: CapacitorConfig = {
  appId: 'com.aeroshutter.app',
  appName: 'AeroShutter',
  webDir: 'dist',
  backgroundColor: '#0b0e14',
  plugins: {
    SplashScreen: {
      launchShowDuration: 900,
      backgroundColor: '#0b0e14',
      showSpinner: false,
      androidSplashResourceName: 'splash',
    },
    StatusBar: {
      style: 'DARK',
      backgroundColor: '#0b0e14',
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
