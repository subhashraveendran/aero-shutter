import { useEffect } from 'react';
import { useStore } from '../store';
import type { Destination } from '../lib/settings';
import { CheckIcon } from '../components/icons';
import { RELEASES_PAGE_URL, WEB_NATIVE_VERSION } from '../lib/updater';

function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      className={`toggle ${on ? 'on' : ''}`}
      role="switch"
      aria-checked={on}
      onClick={() => onChange(!on)}
    >
      <span className="knob" />
    </button>
  );
}

const DESTINATIONS: { id: Destination; title: string; sub: string }[] = [
  { id: 'gallery', title: 'Phone gallery', sub: 'AeroShutter album (app data folder)' },
  { id: 'files', title: 'Files folder', sub: 'User-visible Documents/AeroShutter' },
  { id: 'off', title: 'Off (browse only)', sub: 'Do not save imported photos' },
];

// Visible, testable "Software update" panel. Shows the running version(s), a
// prominent manual check button, and a status line that reflects the last
// check — so the OTA flow is verifiable on a real device instead of relying
// only on the silent auto-banner.
function SoftwareUpdate() {
  const current = useStore((s) => s.updateCurrentVersion);
  const native = useStore((s) => s.updateNativeVersion);
  const status = useStore((s) => s.updateStatus);
  const latest = useStore((s) => s.updateLatest);
  const error = useStore((s) => s.updateLastError);
  const checking = useStore((s) => s.updateChecking);
  const applying = useStore((s) => s.updateApplying);
  const progress = useStore((s) => s.updateProgress);
  const pending = useStore((s) => s.updatePending);
  const checkManual = useStore((s) => s.checkForUpdatesManual);
  const checkAuto = useStore((s) => s.checkForUpdates);
  const runUpdate = useStore((s) => s.runUpdate);
  const applyPending = useStore((s) => s.applyPendingUpdate);

  // Populate the version fields on mount (auto-check is debounced + demo-safe).
  useEffect(() => {
    void checkAuto();
  }, [checkAuto]);

  const isNativeShell = native !== WEB_NATIVE_VERSION;
  const versionLine = isNativeShell ? `web v${current} · app v${native}` : `web v${current}`;

  const openReleases = () => {
    if (typeof window !== 'undefined') window.open(RELEASES_PAGE_URL, '_blank');
  };

  return (
    <div className="settings-group">
      <div className="group-title">Software update</div>
      <div className="card update-card">
        <div className="update-version">
          <div className="setting-label">
            <strong>Current version</strong>
            <span className="update-ver-line">{versionLine}</span>
          </div>
        </div>

        <button
          className="btn btn-primary btn-block"
          onClick={() => void checkManual()}
          disabled={checking}
        >
          {checking ? (
            <>
              <span className="update-spinner" aria-hidden="true" /> Checking…
            </>
          ) : (
            'Check for updates'
          )}
        </button>

        <div className="update-status" role="status" aria-live="polite">
          {error ? (
            <span className="update-error">{error}</span>
          ) : status === 'ota-available' ? (
            <>
              <span>Update available (v{latest?.version ?? ''})</span>
              {pending ? (
                <button className="btn btn-ghost update-inline-btn" onClick={() => void applyPending()}>
                  Restart to apply
                </button>
              ) : applying ? (
                <span className="update-progress-inline">{progress}%</span>
              ) : (
                <button className="btn btn-ghost update-inline-btn" onClick={() => void runUpdate()}>
                  Update now
                </button>
              )}
            </>
          ) : status === 'native-required' ? (
            <>
              <span>Full update required</span>
              <button className="btn btn-ghost update-inline-btn" onClick={openReleases}>
                Get APK
              </button>
            </>
          ) : (
            <span className="update-uptodate">Up to date</span>
          )}
        </div>
      </div>
    </div>
  );
}

export function SettingsScreen() {
  const settings = useStore((s) => s.settings);
  const update = useStore((s) => s.updateSettings);
  const disconnect = useStore((s) => s.disconnect);
  const demo = useStore((s) => s.demo);
  const cameraModel = useStore((s) => s.cameraModel);

  return (
    <div className="screen">
      <div className="topbar">
        <h1>Settings</h1>
        <div className="spacer" />
        {demo && (
          <span className="badge badge-demo">
            <span className="dot" /> Demo
          </span>
        )}
      </div>

      <div className="scroll stagger">
        <div className="settings-group">
          <div className="group-title">Import destination</div>
          <div className="card dest-seg">
            {DESTINATIONS.map((d) => (
              <div
                key={d.id}
                className={`dest-option ${settings.destination === d.id ? 'on' : ''}`}
                onClick={() => void update({ destination: d.id })}
              >
                <div className={`radio ${settings.destination === d.id ? 'on' : ''}`} />
                <div className="setting-label">
                  <strong>{d.title}</strong>
                  <span>{d.sub}</span>
                </div>
                {settings.destination === d.id && (
                  <CheckIcon size={16} className="" />
                )}
              </div>
            ))}
          </div>
        </div>

        <div className="settings-group">
          <div className="group-title">Auto import</div>
          <div className="card">
            <div className="setting-row">
              <div className="setting-label">
                <strong>Auto-import new shots</strong>
                <span>Import new frames the moment they land, automatically.</span>
              </div>
              <Toggle on={settings.autoImport} onChange={(v) => void update({ autoImport: v })} />
            </div>
            <div className="setting-row">
              <div className="setting-label">
                <strong>Watch mode</strong>
                <span>Poll the camera every 5s for fresh frames while connected.</span>
              </div>
              <Toggle on={settings.watchMode} onChange={(v) => void update({ watchMode: v })} />
            </div>
          </div>
        </div>

        <div className="settings-group">
          <div className="group-title">Connection</div>
          <div className="card">
            <div className="setting-row">
              <div className="setting-label">
                <strong>Camera IP</strong>
                <span>Default 192.168.1.1 for Nikon Wi-Fi</span>
              </div>
              <input
                className="ip-input-inline"
                inputMode="decimal"
                value={settings.cameraIp}
                onChange={(e) => void update({ cameraIp: e.target.value })}
                aria-label="Camera IP"
              />
            </div>
            <div className="setting-row">
              <div className="setting-label">
                <strong>Keep screen awake</strong>
                <span>Prevent sleep while connected.</span>
              </div>
              <Toggle on={settings.keepAwake} onChange={(v) => void update({ keepAwake: v })} />
            </div>
          </div>
        </div>

        <div className="settings-group">
          <div className="group-title">Appearance</div>
          <div className="card setting-row">
            <div className="setting-label">
              <strong>Theme</strong>
              <span>Darkroom, or lightbox contact-sheet.</span>
            </div>
            <div className="segmented" style={{ width: 176 }}>
              <button
                className={settings.theme === 'dark' ? 'active' : ''}
                onClick={() => void update({ theme: 'dark' })}
              >
                Darkroom
              </button>
              <button
                className={settings.theme === 'light' ? 'active' : ''}
                onClick={() => void update({ theme: 'light' })}
              >
                Lightbox
              </button>
            </div>
          </div>
        </div>

        <SoftwareUpdate />

        <div className="settings-group">
          <button className="btn btn-ghost btn-block" onClick={() => void disconnect()}>
            Disconnect{cameraModel ? ` from ${cameraModel}` : ''}
          </button>
        </div>

        <div className="about">
          <p>
            AeroShutter is the phone companion of the{' '}
            <a href="https://github.com/subhashraveendran/aero-shutter" target="_blank" rel="noreferrer">
              aero-shutter
            </a>{' '}
            command-line tool. It speaks the same PTP/IP protocol to browse, import and control
            Wi-Fi Nikon cameras.
          </p>
          <p className="ver">Version 1.0.0</p>
        </div>
      </div>
    </div>
  );
}
