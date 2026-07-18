import { useStore } from '../store';
import { RELEASES_PAGE_URL } from '../lib/updater';

// A tasteful, darkroom/amber OTA update banner. It floats at the bottom above
// the tab bar, is fully dismissible, and never blocks the connect flow. Shows
// nothing on web / demo (updateStatus stays 'up-to-date').
export function UpdateBanner() {
  const status = useStore((s) => s.updateStatus);
  const latest = useStore((s) => s.updateLatest);
  const applying = useStore((s) => s.updateApplying);
  const progress = useStore((s) => s.updateProgress);
  const pending = useStore((s) => s.updatePending);
  const dismissed = useStore((s) => s.updateDismissed);
  const runUpdate = useStore((s) => s.runUpdate);
  const applyPending = useStore((s) => s.applyPendingUpdate);
  const dismiss = useStore((s) => s.dismissUpdate);

  const showOta = status === 'ota-available';
  const showNative = status === 'native-required';
  if ((!showOta && !showNative) || dismissed) return null;

  const version = latest?.version ?? '';
  const notes = latest?.notes ?? '';

  const openReleases = () => {
    if (typeof window !== 'undefined') window.open(RELEASES_PAGE_URL, '_blank');
  };

  return (
    <div className="ota-banner card" role="status" aria-live="polite">
      <button
        className="ota-close"
        aria-label="Dismiss update notice"
        onClick={dismiss}
      >
        ×
      </button>

      {showOta && (
        <>
          <div className="ota-title">
            <span className="badge badge-demo">
              <span className="dot" /> Update
            </span>
            <span>
              Update available{version ? ` (v${version})` : ''}
            </span>
          </div>
          {notes && <p className="ota-notes">{notes}</p>}

          {pending ? (
            <button className="btn btn-primary btn-block" onClick={() => void applyPending()}>
              Restart to apply
            </button>
          ) : applying ? (
            <div className="ota-progress" aria-label="Downloading update">
              <div className="ota-progress-fill" style={{ width: `${progress}%` }} />
              <span className="ota-progress-label">{progress}%</span>
            </div>
          ) : (
            <button className="btn btn-primary btn-block" onClick={() => void runUpdate()}>
              Update now
            </button>
          )}
        </>
      )}

      {showNative && (
        <>
          <div className="ota-title">
            <span className="badge badge-demo">
              <span className="dot" /> Update
            </span>
            <span>A new version requires a full update</span>
          </div>
          <p className="ota-notes">
            This release includes native changes that a live update can’t patch. Install the latest
            APK to continue getting updates.
          </p>
          <button className="btn btn-primary btn-block" onClick={openReleases}>
            Get the new APK
          </button>
        </>
      )}
    </div>
  );
}
