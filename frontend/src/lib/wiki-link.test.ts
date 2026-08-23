import { describe, expect, it } from 'vitest';

import { Schema } from '@milkdown/kit/prose/model';
import { EditorState, type Plugin } from '@milkdown/kit/prose/state';
import type { DecorationSet } from '@milkdown/kit/prose/view';

import { wikiLinkPlugin } from './wiki-link';

const schema = new Schema({
  nodes: {
    doc: { content: 'block+' },
    paragraph: { content: 'inline*', group: 'block', toDOM: () => ['p', 0] },
    text: { group: 'inline' }
  }
});

/** N paragraphes, chacun portant un wiki-link existant et un inconnu. */
function makeDoc(blocks: number) {
  return schema.node(
    'doc',
    null,
    Array.from({ length: blocks }, (_, i) =>
      schema.node(
        'paragraph',
        null,
        schema.text(`Bloc ${i} : voir [[Cible ${i}]] et [[Absent ${i}]] ensuite.`)
      )
    )
  );
}

const knownTitles = (blocks: number) =>
  new Set(Array.from({ length: blocks }, (_, i) => `Cible ${i}`));

function makeState(blocks: number): { state: EditorState; plugin: Plugin } {
  const titles = knownTitles(blocks);
  const plugin = wikiLinkPlugin({
    onNavigate: () => {},
    resolve: () => (target: string) => titles.has(target)
  });
  return {
    plugin,
    state: EditorState.create({ schema, doc: makeDoc(blocks), plugins: [plugin] })
  };
}

type DecorationLike = { from: number; to: number; type: { attrs: Record<string, string> } };

function decorations(state: EditorState, plugin: Plugin): DecorationLike[] {
  const set = plugin.getState(state) as DecorationSet;
  return set.find() as unknown as DecorationLike[];
}

describe('wikiLinkPlugin decorations', () => {
  it('decorates every wiki-link of the initial document once refreshed', () => {
    const { state, plugin } = makeState(5);
    const refreshed = state.apply(state.tr.setMeta('wiki-link-refresh', true));

    const found = decorations(refreshed, plugin);
    expect(found).toHaveLength(10);
    expect(found.filter((d) => d.type.attrs.class.includes('wiki-link--exists'))).toHaveLength(5);
    expect(found.filter((d) => d.type.attrs.class.includes('wiki-link--missing'))).toHaveLength(5);
  });

  it('carries the link target so the click handler can resolve it', () => {
    const { state, plugin } = makeState(2);
    const refreshed = state.apply(state.tr.setMeta('wiki-link-refresh', true));

    expect(decorations(refreshed, plugin).map((d) => d.type.attrs['data-target'])).toEqual([
      'Cible 0',
      'Absent 0',
      'Cible 1',
      'Absent 1'
    ]);
  });

  it('keeps other blocks untouched when a middle block is edited', () => {
    const blocks = 40;
    const { state, plugin } = makeState(blocks);
    const base = state.apply(state.tr.setMeta('wiki-link-refresh', true));
    const before = decorations(base, plugin);

    // Insertion au tout début du bloc du milieu : les décorations qui suivent
    // doivent être décalées de la longueur insérée, pas recalculées de travers.
    const middle = base.doc.resolve(Math.floor(base.doc.content.size / 2));
    const insertAt = middle.before(1) + 1;
    const inserted = 'X';
    const edited = base.apply(base.tr.insertText(inserted, insertAt));
    const after = decorations(edited, plugin);

    expect(after).toHaveLength(before.length);
    const shift = (d: DecorationLike) => (d.from >= insertAt ? inserted.length : 0);
    expect(after.map((d) => [d.from, d.to])).toEqual(
      before.map((d) => [d.from + shift(d), d.to + shift(d)])
    );
  });

  it('picks up a wiki-link typed into an existing block', () => {
    const { state, plugin } = makeState(3);
    const base = state.apply(state.tr.setMeta('wiki-link-refresh', true));

    const insertAt = base.doc.resolve(1).before(1) + 1;
    const edited = base.apply(base.tr.insertText('[[Cible 1]] ', insertAt));

    const found = decorations(edited, plugin);
    expect(found).toHaveLength(7);
    expect(found[0].from).toBe(insertAt);
    expect(found[0].type.attrs['data-target']).toBe('Cible 1');
  });

  it('drops the decoration when its brackets are removed', () => {
    const { state, plugin } = makeState(1);
    const base = state.apply(state.tr.setMeta('wiki-link-refresh', true));
    const text = base.doc.textContent;

    const start = text.indexOf('[[Cible 0]]') + 1;
    const edited = base.apply(base.tr.delete(start, start + 2));

    expect(decorations(edited, plugin)).toHaveLength(1);
  });

  it('leaves decorations alone for a transaction that does not change the doc', () => {
    const { state, plugin } = makeState(3);
    const base = state.apply(state.tr.setMeta('wiki-link-refresh', true));
    const before = plugin.getState(base);

    const edited = base.apply(base.tr.setMeta('unrelated', true));

    expect(plugin.getState(edited)).toBe(before);
  });
});
