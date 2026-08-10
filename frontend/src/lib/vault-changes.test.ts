import { describe, expect, it } from 'vitest';
import { applyVaultChangesPure, upsertSummaries } from './vault-changes';
import { domain } from '../../wailsjs/go/models';

type NoteSummary = domain.NoteSummary;

function summary(path: string, updatedAt: string = '2026-01-01T00:00:00Z'): NoteSummary {
  return {
    relativePath: path,
    title: path.split('/').pop() ?? path,
    updatedAt,
    tags: []
  } as unknown as NoteSummary;
}

describe('applyVaultChangesPure', () => {
  it('fullRescan shortcuts to a refresh signal', () => {
    const notes = [summary('a.md')];
    const r = applyVaultChangesPure(
      { notes, pinned: [] },
      { revision: 1, fullRescan: true, changes: [] }
    );
    expect(r.invalidatesFolders).toBe(true);
    expect(r.needsPinnedAndTagsRefresh).toBe(true);
    expect(r.deletedPaths.length).toBe(0);
  });

  it('removes deleted paths from notes and pinned', () => {
    const notes = [summary('a.md'), summary('b.md')];
    const pinned = [summary('a.md')];
    const r = applyVaultChangesPure(
      { notes, pinned },
      {
        revision: 1,
        fullRescan: false,
        changes: [{ kind: 'delete', path: 'a.md' }]
      }
    );
    expect(r.notes.map((n) => n.relativePath)).toEqual(['b.md']);
    expect(r.pinned).toEqual([]);
    expect(r.deletedPaths).toEqual(['a.md']);
  });

  it('flags the open note as affected if it was deleted', () => {
    const r = applyVaultChangesPure(
      { notes: [], pinned: [], openPath: 'notes/open.md' },
      {
        revision: 1,
        fullRescan: false,
        changes: [{ kind: 'delete', path: 'notes/open.md' }]
      }
    );
    expect(r.openNoteAffected).toBe(true);
  });

  it('marks folders/tags as needing refresh when only an upsert occurs', () => {
    const r = applyVaultChangesPure(
      { notes: [], pinned: [] },
      {
        revision: 1,
        fullRescan: false,
        changes: [{ kind: 'upsert', path: 'notes/new.md' }]
      }
    );
    expect(r.invalidatesFolders).toBe(true);
    expect(r.needsPinnedAndTagsRefresh).toBe(true);
    expect(r.deletedPaths.length).toBe(0);
  });
});

describe('upsertSummaries', () => {
  it('replaces existing entries and adds new ones, sorted by updatedAt desc', () => {
    const original = [summary('a.md', '2026-01-01T00:00:00Z')];
    const fetched = [
      summary('a.md', '2026-02-01T00:00:00Z'),
      summary('b.md', '2026-03-01T00:00:00Z')
    ];
    const next = upsertSummaries(original, fetched);
    expect(next.map((n) => n.relativePath)).toEqual(['b.md', 'a.md']);
  });
});
