// Pure helpers for the in-app intervalometer. Kept side-effect free so the
// interval math can be unit-tested without timers or the store.

export interface TimelapsePlan {
  /** Seconds between shutter releases. */
  intervalSec: number;
  /** Total shots to take, or 0 / null for "until stopped". */
  totalShots: number | null;
}

/** Clamp user input into a sane intervalometer plan. */
export function normalizePlan(intervalSec: number, totalShots: number | null): TimelapsePlan {
  const interval = Math.max(1, Math.round(intervalSec) || 1);
  let total: number | null = totalShots;
  if (total !== null) {
    total = Math.max(1, Math.round(total) || 1);
  }
  return { intervalSec: interval, totalShots: total };
}

/** True when the sequence has taken all requested shots. */
export function isComplete(plan: TimelapsePlan, shotsTaken: number): boolean {
  return plan.totalShots !== null && shotsTaken >= plan.totalShots;
}

/**
 * Total planned duration in seconds for a finite sequence. The first frame
 * fires immediately, so duration spans the gaps between the remaining frames:
 * (n - 1) * interval. Returns null for open-ended sequences.
 */
export function plannedDurationSec(plan: TimelapsePlan): number | null {
  if (plan.totalShots === null) return null;
  return Math.max(0, plan.totalShots - 1) * plan.intervalSec;
}

/** Estimated seconds remaining given shots already taken. */
export function remainingSec(plan: TimelapsePlan, shotsTaken: number, secToNext: number): number | null {
  if (plan.totalShots === null) return null;
  const shotsLeft = Math.max(0, plan.totalShots - shotsTaken);
  if (shotsLeft <= 0) return 0;
  // secToNext covers the current gap; each further shot adds a full interval.
  return Math.round(secToNext + Math.max(0, shotsLeft - 1) * plan.intervalSec);
}

/** Format a whole number of seconds as m:ss (or h:mm:ss when long). */
export function formatDuration(totalSec: number): string {
  const s = Math.max(0, Math.round(totalSec));
  const hours = Math.floor(s / 3600);
  const mins = Math.floor((s % 3600) / 60);
  const secs = s % 60;
  const pad = (n: number) => String(n).padStart(2, '0');
  if (hours > 0) return `${hours}:${pad(mins)}:${pad(secs)}`;
  return `${mins}:${pad(secs)}`;
}
