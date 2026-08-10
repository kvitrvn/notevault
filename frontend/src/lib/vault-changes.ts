import { domain } from '../../wailsjs/go/models';

type NoteSummary = domain.NoteSummary;

export type VaultChange = { kind: 'upsert' | 'delete'; path: string };
export type VaultChangeBatch = {
  revision: number;
  changes: VaultChange[];
  fullRescan: boolean;
};

export type ApplyResult = {
  notes: NoteSummary[];
  pinned: NoteSummary[];
  deletedPaths: string[];
  invalidatesFolders: boolean;
  needsPinnedAndTagsRefresh: boolean;
  openNoteAffected: boolean;
};

export function applyVaultChangesPure(
  current: { notes: NoteSummary[]; pinned: NoteSummary[]; openPath?: string },
  batch: VaultChangeBatch
): ApplyResult {
  if (batch.fullRescan) {
    return {
      notes: current.notes,
      pinned: current.pinned,
      deletedPaths: [],
      invalidatesFolders: true,
      needsPinnedAndTagsRefresh: true,
      openNoteAffected: false
    };
  }
  const upserts = batch.changes.filter((c) => c.kind === 'upsert').map((c) => c.path);
  const deletes = batch.changes.filter((c) => c.kind === 'delete').map((c) => c.path);
  const delSet = new Set(deletes);
  const nextNotes = current.notes.filter((n) => !delSet.has(n.relativePath));
  const nextPinned = current.pinned.filter((n) => !delSet.has(n.relativePath));
  return {
    notes: nextNotes,
    pinned: nextPinned,
    deletedPaths: deletes,
    invalidatesFolders: deletes.length + upserts.length > 0,
    needsPinnedAndTagsRefresh: deletes.length + upserts.length > 0,
    openNoteAffected: current.openPath !== undefined && delSet.has(current.openPath)
  };
}

export function upsertSummaries(
  notes: NoteSummary[],
  fetched: NoteSummary[]
): NoteSummary[] {
  const fetchedMap = new Map(fetched.map((n) => [n.relativePath, n]));
  const existing = new Set(notes.map((n) => n.relativePath));
  const additions: NoteSummary[] = [];
  for (const f of fetched) {
    if (!existing.has(f.relativePath)) {
      additions.push(f);
    }
  }
  const merged = [...notes.map((n) => fetchedMap.get(n.relativePath) ?? n), ...additions];
  merged.sort((a, b) => {
    const ua = String(a.updatedAt ?? '');
    const ub = String(b.updatedAt ?? '');
    if (ua !== ub) return ub.localeCompare(ua);
    return a.relativePath.localeCompare(b.relativePath);
  });
  return merged;
}
