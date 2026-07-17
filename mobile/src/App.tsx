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

export function App() {
  const init = useStore((s) => s.init);
  const connected = useStore((s) => s.connected);
  const screen = useStore((s) => s.screen);
  const demo = useStore((s) => s.demo);

  useEffect(() => {
    void init();
  }, [init]);

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

      <Toasts />
    </div>
  );
}
