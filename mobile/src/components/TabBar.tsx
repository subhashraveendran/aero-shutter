import { useStore, type Screen } from '../store';
import { CameraIcon, DownloadIcon, GridIcon, SettingsIcon, SlidersIcon } from './icons';
import { tap } from '../lib/haptics';

const TABS: { id: Screen; label: string; icon: React.ReactNode }[] = [
  { id: 'gallery', label: 'Sheet', icon: <GridIcon size={20} className="" /> },
  { id: 'control', label: 'Exposure', icon: <SlidersIcon size={20} className="" /> },
  { id: 'import', label: 'Develop', icon: <DownloadIcon size={20} className="" /> },
  { id: 'settings', label: 'Setup', icon: <SettingsIcon size={20} className="" /> },
];

export function TabBar() {
  const screen = useStore((s) => s.screen);
  const navigate = useStore((s) => s.navigate);
  const queueCount = useStore((s) => s.importQueue.length);
  const importing = useStore((s) => s.importing);

  return (
    <nav className="tabbar">
      {TABS.map((t) => {
        const active = screen === t.id;
        return (
          <button
            key={t.id}
            className={`tab ${active ? 'active' : ''}`}
            onClick={() => {
              void tap();
              navigate(t.id);
            }}
            aria-current={active ? 'page' : undefined}
          >
            <span className="tab-icon">
              {t.id === 'gallery' && !active ? <CameraIcon size={20} className="" /> : t.icon}
            </span>
            <span>
              {t.label}
              {t.id === 'import' && (importing || queueCount > 0) ? ` (${queueCount})` : ''}
            </span>
          </button>
        );
      })}
    </nav>
  );
}
