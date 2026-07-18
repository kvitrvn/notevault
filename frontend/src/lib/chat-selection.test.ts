import { describe, expect, it } from 'vitest';

import { resolveChatPaths } from './chat-selection';

const notes = [
  { relativePath: 'notes/a.md', tags: ['dpo', 'aster'] },
  { relativePath: 'notes/b.md', tags: ['rd'] },
  { relativePath: 'notes/c.md', tags: ['dpo', 'rd'] },
  { relativePath: 'notes/d.md', tags: [] }
];

describe('resolveChatPaths', () => {
  it('additionne les notes de plusieurs tags', () => {
    expect(resolveChatPaths(notes, [], ['dpo', 'rd'], [])).toEqual([
      'notes/a.md',
      'notes/b.md',
      'notes/c.md'
    ]);
  });

  it('conserve les ajouts manuels et retire les exceptions', () => {
    expect(resolveChatPaths(notes, ['notes/d.md'], ['dpo'], ['notes/c.md'])).toEqual([
      'notes/a.md',
      'notes/d.md'
    ]);
  });

  it('ignore les chemins manuels qui ne sont plus dans le coffre', () => {
    expect(resolveChatPaths(notes, ['notes/absente.md'], [], [])).toEqual([]);
  });
});
