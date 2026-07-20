// Extension Tiptap qui surligne les motifs [[Titre]] en wiki-links cliquables.
//
// Approche : décorations ProseMirror. Le texte `[[Titre]]` reste tel quel
// dans le document (donc préservé par la sérialisation Markdown), seule
// l'apparence change (souligné + couleur d'accent) via une Decoration.inline
// qui wrappe le texte dans une balise <a class="wiki-link">.
//
// Les titres de notes existantes sont marqués en bleu accent, les titres
// inconnus en rouge (avec une classe distincte). Un click sur le lien
// émet un callback fourni par l'hôte.

import { Extension, type Editor } from '@tiptap/core';
import { Plugin, PluginKey, type Transaction } from '@tiptap/pm/state';
import { Decoration, DecorationSet } from '@tiptap/pm/view';
import type { Node as PMNode } from '@tiptap/pm/model';

export type WikiLinkClickHandler = (target: string) => void;
export type WikiLinkCreateHandler = (target: string) => void;
export type WikiLinkResolve = (target: string) => boolean;

export type WikiLinkOptions = {
  /** Appelé pour naviguer vers la note liée. */
  onNavigate: WikiLinkClickHandler;
  /** Appelé pour créer une note si la cible n'existe pas. */
  onCreate?: WikiLinkCreateHandler;
  /** Getter dynamique appelé à chaque apply. */
  resolve: () => WikiLinkResolve;
};

const WIKI_LINK_RE = /\[\[([^\]\n]+?)\]\]/g;
const PLUGIN_KEY = new PluginKey('wiki-link');

export const WikiLink = Extension.create<WikiLinkOptions>({
  name: 'wikiLink',

  addOptions() {
    return {
      onNavigate: () => {},
      onCreate: undefined,
      resolve: () => () => true
    };
  },

  addProseMirrorPlugins() {
    const opts = this.options;
    return [
      new Plugin({
        key: PLUGIN_KEY,
        state: {
          init: () => DecorationSet.empty,
          apply(tr, old) {
            if (!tr.docChanged && !tr.getMeta('wiki-link-refresh')) return old;
            if (tr.getMeta('wiki-link-refresh')) {
              return buildDecorations(tr.doc, opts.resolve());
            }
            return applyIncremental(tr, old, opts.resolve());
          }
        },
        props: {
          decorations(state) {
            return PLUGIN_KEY.getState(state) as DecorationSet;
          },
          handleClick(view, pos, event) {
            const target = event.target as HTMLElement | null;
            if (!target) return false;
            const link = target.closest('.wiki-link') as HTMLElement | null;
            if (!link) return false;
            const targetName = link.getAttribute('data-target') ?? '';
            if (!targetName) return false;
            event.preventDefault();
            if (event.metaKey || event.ctrlKey) {
              opts.onCreate?.(targetName);
            } else {
              opts.onNavigate(targetName);
            }
            return true;
          }
        }
      })
    ];
  }
});

function applyIncremental(
  tr: Transaction,
  old: DecorationSet,
  resolve: WikiLinkResolve
): DecorationSet {
  let set = old.map(tr.mapping, tr.doc);

  const touched = new Map<string, { from: number; to: number }>();
  for (const step of tr.steps) {
    const stepMap = step.getMap();
    stepMap.forEach((_oldStart, _oldEnd, newStart, newEnd) => {
      if (newEnd > newStart) addTouchedBlock(tr.doc, newStart, touched);
      if (newEnd > newStart && newEnd !== newStart) {
        addTouchedBlock(tr.doc, newEnd, touched);
      }
    });
  }

  if (touched.size === 0) return set;

  const toRemove: Decoration[] = [];
  for (const block of touched.values()) {
    toRemove.push(...set.find(block.from, block.to));
  }
  if (toRemove.length > 0) {
    set = set.remove(toRemove);
  }
  for (const block of touched.values()) {
    set = set.add(tr.doc, findWikiLinksInRange(tr.doc, block.from, block.to, resolve));
  }

  return set;
}

function addTouchedBlock(
  doc: PMNode,
  pos: number,
  touched: Map<string, { from: number; to: number }>
): void {
  if (pos <= 0 || pos >= doc.content.size) return;
  doc.forEach((child, childPos) => {
    if (!child.isBlock) return;
    const end = childPos + child.nodeSize;
    if (pos >= childPos && pos <= end) {
      touched.set(`${childPos}-${end}`, { from: childPos, to: end });
    }
  });
}

function findWikiLinksInRange(
  doc: PMNode,
  from: number,
  to: number,
  resolve: WikiLinkResolve
): Decoration[] {
  const decorations: Decoration[] = [];
  doc.nodesBetween(from, to, (node, pos) => {
    if (!node.isText) return;
    const text = node.text ?? '';
    const localFrom = Math.max(0, from - pos);
    const localTo = Math.min(text.length, to - pos);
    if (localFrom >= localTo) return;
    const slice = text.slice(localFrom, localTo);
    WIKI_LINK_RE.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = WIKI_LINK_RE.exec(slice)) !== null) {
      const [whole, title] = match;
      const matchFrom = pos + localFrom + match.index;
      const matchTo = matchFrom + whole.length;
      if (matchFrom < from || matchTo > to) continue;
      const exists = resolve(title);
      const cls = exists ? 'wiki-link wiki-link--exists' : 'wiki-link wiki-link--missing';
      decorations.push(
        Decoration.inline(matchFrom, matchTo, {
          class: cls,
          'data-target': title
        })
      );
    }
  });
  return decorations;
}

function buildDecorations(doc: PMNode, resolve: WikiLinkResolve): DecorationSet {
  const decorations: Decoration[] = [];
  doc.descendants((node, pos) => {
    if (!node.isText) return;
    const text = node.text ?? '';
    WIKI_LINK_RE.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = WIKI_LINK_RE.exec(text)) !== null) {
      const [whole, title] = match;
      const from = pos + match.index;
      const to = from + whole.length;
      const exists = resolve(title);
      const cls = exists ? 'wiki-link wiki-link--exists' : 'wiki-link wiki-link--missing';
      decorations.push(
        Decoration.inline(from, to, {
          class: cls,
          'data-target': title
        })
      );
    }
  });
  return DecorationSet.create(doc, decorations);
}

// refreshWikiLinkDecorations force le plugin `wikiLink` à recalculer ses
// décorations après un changement de `knownTitles`. On dispatch une
// transaction meta-only ; sans `addToHistory: false`, prosemirror-history
// empile ces transactions dans la pile d'undo et un Ctrl+Z défait d'abord
// ces entrées invisibles avant d'atteindre la saisie de l'utilisateur.
export function refreshWikiLinkDecorations(editor: Editor): void {
  if (!editor) return;
  editor.view.dispatch(
    editor.state.tr.setMeta('wiki-link-refresh', true).setMeta('addToHistory', false)
  );
}