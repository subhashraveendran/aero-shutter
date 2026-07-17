import { describe, expect, it } from 'vitest';
import { DEFAULT_SETTINGS } from './settings';

describe('settings defaults', () => {
  it('includes the darkroom theme + watch-mode fields', () => {
    expect(DEFAULT_SETTINGS.theme).toBe('dark');
    expect(DEFAULT_SETTINGS.watchMode).toBe(true);
    expect(DEFAULT_SETTINGS.autoImport).toBe(false);
    expect(DEFAULT_SETTINGS.destination).toBe('files');
  });
});
