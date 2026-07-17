import { describe, expect, it } from 'vitest';
import {
  formatDuration,
  isComplete,
  normalizePlan,
  plannedDurationSec,
  remainingSec,
} from './timelapse';

describe('timelapse plan math', () => {
  it('clamps interval and shot count to sane whole numbers', () => {
    expect(normalizePlan(0, 0)).toEqual({ intervalSec: 1, totalShots: 1 });
    expect(normalizePlan(4.6, 12.2)).toEqual({ intervalSec: 5, totalShots: 12 });
    expect(normalizePlan(-3, null)).toEqual({ intervalSec: 1, totalShots: null });
  });

  it('detects completion only for finite sequences', () => {
    const finite = normalizePlan(5, 10);
    expect(isComplete(finite, 9)).toBe(false);
    expect(isComplete(finite, 10)).toBe(true);
    expect(isComplete(finite, 11)).toBe(true);
    const openEnded = normalizePlan(5, null);
    expect(isComplete(openEnded, 9999)).toBe(false);
  });

  it('computes planned duration across the gaps (n-1 intervals)', () => {
    expect(plannedDurationSec(normalizePlan(5, 10))).toBe(45); // 9 gaps * 5s
    expect(plannedDurationSec(normalizePlan(2, 1))).toBe(0); // single shot
    expect(plannedDurationSec(normalizePlan(5, null))).toBeNull();
  });

  it('estimates remaining seconds from shots taken and next-shot countdown', () => {
    const plan = normalizePlan(10, 6);
    // taken 2, 3s to next, 3 shots still pending after this gap -> 3 + 3*10
    expect(remainingSec(plan, 2, 3)).toBe(33);
    // last shot pending, 4s to next
    expect(remainingSec(plan, 5, 4)).toBe(4);
    // completed
    expect(remainingSec(plan, 6, 0)).toBe(0);
    expect(remainingSec(normalizePlan(5, null), 100, 2)).toBeNull();
  });

  it('formats durations as m:ss and h:mm:ss', () => {
    expect(formatDuration(0)).toBe('0:00');
    expect(formatDuration(9)).toBe('0:09');
    expect(formatDuration(75)).toBe('1:15');
    expect(formatDuration(3661)).toBe('1:01:01');
  });
});
