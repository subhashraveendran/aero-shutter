// Local notifications for import lifecycle events.
//
// Thin wrapper over @capacitor/local-notifications that no-ops gracefully on
// web / demo (where there is no native scheduler) and swallows permission
// denials so a missing grant never breaks the import flow.

import { isDemoMode } from '@aero-shutter/tcp-socket';
import { LocalNotifications } from '@capacitor/local-notifications';

let permissionGranted: boolean | null = null;
let idSeq = 1;

/** True only when running in a native shell where notifications are possible. */
function available(): boolean {
  return !isDemoMode();
}

/**
 * Ask for notification permission (idempotent). Result is cached for the
 * session. Safe to call on web — returns false without prompting.
 */
export async function requestNotificationPermission(): Promise<boolean> {
  if (!available()) return false;
  if (permissionGranted !== null) return permissionGranted;
  try {
    const status = await LocalNotifications.checkPermissions();
    let granted = status.display === 'granted';
    if (!granted && (status.display === 'prompt' || status.display === 'prompt-with-rationale')) {
      const req = await LocalNotifications.requestPermissions();
      granted = req.display === 'granted';
    }
    permissionGranted = granted;
    return granted;
  } catch {
    permissionGranted = false;
    return false;
  }
}

async function fire(title: string, body: string): Promise<void> {
  if (!available()) return;
  const granted = await requestNotificationPermission();
  if (!granted) return;
  try {
    await LocalNotifications.schedule({
      notifications: [
        {
          id: idSeq++,
          title,
          body,
          // Fire immediately; a small schedule offset avoids "in the past" drops.
          schedule: { at: new Date(Date.now() + 50) },
        },
      ],
    });
  } catch {
    // Never let a notification failure surface to the import flow.
  }
}

/** "Imported 12 photos" — fired when a batch import finishes. */
export async function notifyImportComplete(count: number): Promise<void> {
  if (count <= 0) return;
  await fire('Import complete', `Imported ${count} photo${count === 1 ? '' : 's'}`);
}

/** "New shot: DSC_0123.JPG" — fired when watch/auto-import spots a new frame. */
export async function notifyNewPhoto(filename: string): Promise<void> {
  await fire('New shot', `New shot: ${filename}`);
}

/** Optional: fired once the camera handshake completes. */
export async function notifyCameraConnected(model: string): Promise<void> {
  await fire('Camera connected', `Connected to ${model || 'camera'}`);
}
