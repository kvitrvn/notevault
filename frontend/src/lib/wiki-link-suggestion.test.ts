import { beforeEach, describe, expect, it, vi } from 'vitest';
import { exitSuggestion } from '@tiptap/suggestion';
import type { Editor } from '@tiptap/core';
import {
  completeWikiLinkSuggestion,
  shouldShowWikiLinkSuggestion
} from './wiki-link-suggestion';

vi.mock('@tiptap/suggestion', () => ({
  default: vi.fn(),
  exitSuggestion: vi.fn()
}));

describe('wiki-link suggestion completion', () => {
  beforeEach(() => vi.clearAllMocks());

  it('stops matching once the wiki-link is closed', () => {
    expect(shouldShowWikiLinkSuggestion('Première note')).toBe(true);
    expect(shouldShowWikiLinkSuggestion('Première note]]')).toBe(false);
  });

  it('inserts the link and exits the keyboard-capturing suggestion', () => {
    const run = vi.fn();
    const insertContentAt = vi.fn(() => ({ run }));
    const focus = vi.fn(() => ({ insertContentAt }));
    const chain = vi.fn(() => ({ focus }));
    const view = {};
    const editor = { chain, view } as unknown as Editor;
    const range = { from: 1, to: 8 };

    completeWikiLinkSuggestion(editor, range, 'Cible');

    expect(insertContentAt).toHaveBeenCalledWith(range, '[[Cible]]');
    expect(run).toHaveBeenCalledOnce();
    expect(exitSuggestion).toHaveBeenCalledWith(view, expect.anything());
  });
});
