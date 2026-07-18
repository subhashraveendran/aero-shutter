// Tiny semver comparison util for the OTA updater. We only need to compare
// simple `MAJOR.MINOR.PATCH` versions (optionally prefixed with `v` and/or
// carrying a pre-release/build suffix we ignore). This intentionally avoids
// pulling in a full semver dependency.

/** Parse a version string into [major, minor, patch]; unknown parts => 0. */
export function parseVersion(v: string): [number, number, number] {
  const cleaned = String(v)
    .trim()
    .replace(/^v/i, '')
    // Drop any pre-release / build metadata (e.g. "1.2.3-web", "1.2.3+abc").
    .split(/[-+]/)[0];
  const parts = cleaned.split('.').map((p) => {
    const n = Number.parseInt(p, 10);
    return Number.isFinite(n) ? n : 0;
  });
  return [parts[0] || 0, parts[1] || 0, parts[2] || 0];
}

/**
 * Compare two versions. Returns:
 *  -1 if a < b, 0 if equal, 1 if a > b.
 */
export function compareVersions(a: string, b: string): -1 | 0 | 1 {
  const pa = parseVersion(a);
  const pb = parseVersion(b);
  for (let i = 0; i < 3; i++) {
    if (pa[i] > pb[i]) return 1;
    if (pa[i] < pb[i]) return -1;
  }
  return 0;
}

/** True when `candidate` is strictly newer than `current`. */
export function isNewer(candidate: string, current: string): boolean {
  return compareVersions(candidate, current) === 1;
}

/** True when `version` satisfies the minimum required version (>=). */
export function satisfiesMin(version: string, min: string): boolean {
  return compareVersions(version, min) >= 0;
}
