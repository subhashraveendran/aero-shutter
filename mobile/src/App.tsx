import { useEffect } from 'react';
import { useStore } from './store';
import { ConnectScreen } from './screens/ConnectScreen';
import { GalleryScreen } from './screens/GalleryScreen';
import { DetailScreen } from './screens/DetailScreen';
import { ImportScreen } from './screens/ImportScreen';
import { ControlScreen } from './screens/ControlScreen';
import { SettingsScreen } from './screens/SettingsScreen';
import { TabBar } from './components/TabBar';
import { Toasts } from './components/Toasts';
import { UpdateBanner } from './components/UpdateBanner';

export function App() {
  const init = useStore((s) => s.init);
  const connected = useStore((s) => s.connected);
  const screen = useStore((s) => s.screen);
  const demo = useStore((s) => s.demo);
  const checkForUpdates = useStore((s) => s.checkForUpdates);

  useEffect(() => {
    void init();
  }, [init]);

  // Re-check the OTA manifest whenever the app returns to the foreground. The
  // check itself is debounced and no-ops on web/demo. @capacitor/app is loaded
  // lazily so the browser bundle never depends on the native plugin.
  useEffect(() => {
    let remove: (() => void) | undefined;
    void (async () => {
      try {
        const cap = (globalThis as { Capacitor?: { isNativePlatform?: () => boolean } }).Capacitor;
        if (!cap?.isNativePlatform?.()) return;
        const { App: CapApp } = await import('@capacitor/app');
        const handle = await CapApp.addListener('resume', () => {
          void checkForUpdates();
        });
        remove = () => void handle.remove();
      } catch {
        /* not available */
      }
    })();
    return () => remove?.();
  }, [checkForUpdates]);

  return (
    <div className="app-shell">
      <div className="grain" aria-hidden="true" />
      {demo && connected && (
        <div
          style={{
            position: 'fixed',
            top: 'calc(var(--safe-top) + 6px)',
            right: 12,
            zIndex: 50,
          }}
        >
          <span className="badge badge-demo">
            <span className="dot" /> Demo mode
          </span>
        </div>
      )}

      {!connected ? (
        <ConnectScreen />
      ) : screen === 'detail' ? (
        <DetailScreen />
      ) : (
        <>
          {screen === 'gallery' && <GalleryScreen />}
          {screen === 'control' && <ControlScreen />}
          {screen === 'import' && <ImportScreen />}
          {screen === 'settings' && <SettingsScreen />}
          <TabBar />
        </>
      )}

      <UpdateBanner />
      <Toasts />
    </div>
  );
}
