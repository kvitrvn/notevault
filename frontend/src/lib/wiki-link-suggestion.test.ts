import { describe, expect, it, vi } from 'vitest';
import type { EditorView } from '@milkdown/kit/prose/view';
import {
  completeWikiLinkSuggestion,
  findWikiLinkQuery,
  rankWikiLinkTitles,
  shouldShowWikiLinkSuggestion
} from './wiki-link-suggestion';

describe('wiki-link suggestion matching', () => {
  it('stops matching once the wiki-link is closed', () => {
    expect(shouldShowWikiLinkSuggestion('Première note')).toBe(true);
    expect(shouldShowWikiLinkSuggestion('Première note]]')).toBe(false);
  });

  it('extracts the query after the last unclosed [[', () => {
    expect(findWikiLinkQuery('Voir [[Prem')).toBe('Prem');
    expect(findWikiLinkQuery('[[')).toBe('');
    expect(findWikiLinkQuery('Voir [[Une]] puis [[Deux')).toBe('Deux');
  });

  it('returns null when there is nothing to complete', () => {
    expect(findWikiLinkQuery('texte normal')).toBeNull();
    expect(findWikiLinkQuery('Voir [[Une]] fini')).toBeNull();
    expect(findWikiLinkQuery(`[[${'x'.repeat(200)}`)).toBeNull();
  });
});

describe('rankWikiLinkTitles', () => {
  const titles = ['Alpha', 'Beta', 'alphabet', 'Gamma alpha'];

  it('returns the first titles when the query is empty', () => {
    expect(rankWikiLinkTitles(titles, '')).toEqual(titles);
  });

  it('puts prefix matches before substring matches, case-insensitively', () => {
    expect(rankWikiLinkTitles(titles, 'alpha')).toEqual(['Alpha', 'alphabet', 'Gamma alpha']);
  });

  it('caps the list at 8 items', () => {
    const many = Array.from({ length: 20 }, (_, i) => `Note ${i}`);
    expect(rankWikiLinkTitles(many, 'note')).toHaveLength(8);
  });
});

describe('completeWikiLinkSuggestion', () => {
  it('replaces the pending query with the full wiki-link and refocuses', () => {
    const insertText = vi.fn(() => 'transaction');
    const dispatch = vi.fn();
    const focus = vi.fn();
    const view = {
      state: { tr: { insertText } },
      dispatch,
      focus
    } as unknown as EditorView;

    completeWikiLinkSuggestion(view, { from: 5, to: 12 }, 'Cible');

    expect(insertText).toHaveBeenCalledWith('[[Cible]]', 5, 12);
    expect(dispatch).toHaveBeenCalledWith('transaction');
    expect(focus).toHaveBeenCalledOnce();
  });
});
