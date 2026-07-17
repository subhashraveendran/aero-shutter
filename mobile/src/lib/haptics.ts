import { Haptics, ImpactStyle, NotificationType } from '@capacitor/haptics';

export async function tap(style: ImpactStyle = ImpactStyle.Light): Promise<void> {
  try {
    await Haptics.impact({ style });
  } catch {
    // Not available on this platform.
  }
}

export async function success(): Promise<void> {
  try {
    await Haptics.notification({ type: NotificationType.Success });
  } catch {
    /* noop */
  }
}

export async function warn(): Promise<void> {
  try {
    await Haptics.notification({ type: NotificationType.Warning });
  } catch {
    /* noop */
  }
}

export { ImpactStyle };
