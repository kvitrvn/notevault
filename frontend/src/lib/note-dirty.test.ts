import { describe, expect, it } from 'vitest';

import { domain } from '../../wailsjs/go/models';
import {
  noteIdentity,
  noteMatchesIdentity,
  sameNoteIdentity,
  sameTitleSet
} from './note-dirty';

function note(overrides: Partial<domain.Note> = {}): domain.Note {
  return domain.Note.createFrom({
    relativePath: 'notes/a.md',
    title: 'Titre',
    content: 'Corps',
    tags: ['x', 'y'],
    ...overrides
  });
}

describe('noteIdentity', () => {
  it('keeps only the fields that make a note dirty', () => {
    expect(noteIdentity(note())).toEqual({
      title: 'Titre',
      content: 'Corps',
      tags: ['x', 'y']
    });
  });

  it('copies tags so later in-place mutation does not rewrite the identity', () => {
    const source = note();
    const identity = noteIdentity(source);
    source.tags.push('z');

    expect(identity.tags).toEqual(['x', 'y']);
  });

  it('tolerates a missing tag list', () => {
    const identity = noteIdentity(note({ tags: undefined as unknown as string[] }));

    expect(identity.tags).toEqual([]);
  });
});

describe('sameNoteIdentity', () => {
  it('treats two identities with the same fields as equal', () => {
    expect(sameNoteIdentity(noteIdentity(note()), noteIdentity(note()))).toBe(true);
  });

  it('detects a content change', () => {
    expect(sameNoteIdentity(noteIdentity(note()), noteIdentity(note({ content: 'Autre' })))).toBe(
      false
    );
  });

  it('detects a title change', () => {
    expect(sameNoteIdentity(noteIdentity(note()), noteIdentity(note({ title: 'Autre' })))).toBe(
      false
    );
  });

  it('detects an added, removed or reordered tag', () => {
    const base = noteIdentity(note());

    expect(sameNoteIdentity(base, noteIdentity(note({ tags: ['x', 'y', 'z'] })))).toBe(false);
    expect(sameNoteIdentity(base, noteIdentity(note({ tags: ['x'] })))).toBe(false);
    expect(sameNoteIdentity(base, noteIdentity(note({ tags: ['y', 'x'] })))).toBe(false);
  });

  it('handles null on either side', () => {
    expect(sameNoteIdentity(null, null)).toBe(true);
    expect(sameNoteIdentity(noteIdentity(note()), null)).toBe(false);
    expect(sameNoteIdentity(null, noteIdentity(note()))).toBe(false);
  });

  it('does not confuse an empty note with a missing one', () => {
    const empty = { title: '', content: '', tags: [] };

    expect(sameNoteIdentity(empty, null)).toBe(false);
  });
});

describe('noteMatchesIdentity', () => {
  it('matches a note against the identity captured from it', () => {
    const source = note();

    expect(noteMatchesIdentity(source, noteIdentity(source))).toBe(true);
  });

  it('detects a content, title or tag change', () => {
    const saved = noteIdentity(note());

    expect(noteMatchesIdentity(note({ content: 'Autre' }), saved)).toBe(false);
    expect(noteMatchesIdentity(note({ title: 'Autre' }), saved)).toBe(false);
    expect(noteMatchesIdentity(note({ tags: ['x'] }), saved)).toBe(false);
    expect(noteMatchesIdentity(note({ tags: ['y', 'x'] }), saved)).toBe(false);
  });

  it('treats a missing tag list as empty', () => {
    const saved = noteIdentity(note({ tags: [] }));

    expect(noteMatchesIdentity(note({ tags: undefined as unknown as string[] }), saved)).toBe(true);
  });

  // `lastSavedIdentity = null` est la sentinelle « forcer la prochaine
  // sauvegarde » : une note ouverte ne doit jamais y correspondre.
  it('never matches the null sentinel for an open note', () => {
    expect(noteMatchesIdentity(note(), null)).toBe(false);
    expect(noteMatchesIdentity(null, noteIdentity(note()))).toBe(false);
    expect(noteMatchesIdentity(null, null)).toBe(true);
  });
});

describe('sameTitleSet', () => {
  it('is true for the same reference', () => {
    const set = new Set(['a']);

    expect(sameTitleSet(set, set)).toBe(true);
  });

  it('is true for distinct sets with the same members, whatever the order', () => {
    expect(sameTitleSet(new Set(['a', 'b']), new Set(['b', 'a']))).toBe(true);
  });

  it('is false when a title is added, removed or replaced', () => {
    expect(sameTitleSet(new Set(['a']), new Set(['a', 'b']))).toBe(false);
    expect(sameTitleSet(new Set(['a', 'b']), new Set(['a']))).toBe(false);
    expect(sameTitleSet(new Set(['a', 'b']), new Set(['a', 'c']))).toBe(false);
  });

  it('is true for two empty sets', () => {
    expect(sameTitleSet(new Set(), new Set())).toBe(true);
  });
});
