// Dexie (IndexedDB) ledger mirroring the CLI's SQLite dedupe table:
// object handle (+ a stable identity key) -> imported record.

import Dexie, { type Table } from 'dexie';

export interface ImportedRecord {
  /** Stable identity: filename + size, resilient across reconnects where the
   * object handle may be reassigned. */
  id: string;
  handle: number;
  filename: string;
  size: number;
  format: string;
  importedAt: number;
  destination: string;
  path: string;
}

class AeroShutterDb extends Dexie {
  imported!: Table<ImportedRecord, string>;

  constructor() {
    super('aero-shutter');
    this.version(1).stores({
      imported: 'id, handle, filename, importedAt',
    });
  }
}

export const db = new AeroShutterDb();

export function identityKey(filename: string, size: number): string {
  return `${filename}:${size}`;
}

export async function isImported(filename: string, size: number): Promise<boolean> {
  const rec = await db.imported.get(identityKey(filename, size));
  return !!rec;
}

export async function markImported(rec: Omit<ImportedRecord, 'id'>): Promise<void> {
  await db.imported.put({ ...rec, id: identityKey(rec.filename, rec.size) });
}

export async function importedToday(): Promise<number> {
  const start = new Date();
  start.setHours(0, 0, 0, 0);
  return db.imported.where('importedAt').aboveOrEqual(start.getTime()).count();
}

export async function allImportedIds(): Promise<Set<string>> {
  const rows = await db.imported.toArray();
  return new Set(rows.map((r) => r.id));
}
