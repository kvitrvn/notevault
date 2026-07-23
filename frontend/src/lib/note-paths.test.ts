import { describe, expect, it } from 'vitest';
import { normalizeNotesFolderPath } from './note-paths';

describe('normalizeNotesFolderPath', () => {
  it.each(['', '/', 'notes', 'notes/'])('keeps the notes root canonical: %j', (path) => {
    expect(normalizeNotesFolderPath(path)).toBe('notes');
  });

  it.each([
    ['projets', 'notes/projets'],
    ['projets/web/', 'notes/projets/web'],
    ['notes/projets', 'notes/projets'],
    ['/notes//projets/', 'notes/projets'],
    ['notes\\projets\\web', 'notes/projets/web']
  ])('normalizes %j to %j', (path, expected) => {
    expect(normalizeNotesFolderPath(path)).toBe(expected);
  });
});
