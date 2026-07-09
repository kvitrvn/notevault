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
import { Plugin, PluginKey } from '@tiptap/pm/state';
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
            return buildDecorations(tr.doc, opts.resolve());
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

function buildDecorations(doc: PMNode, resolve: WikiLinkResolve): DecorationSet {
  const decorations: Decoration[] = [];
  doc.descendants((node, pos) => {
    if (!node.isText) return;
    const text = node.text ?? '';
    let match: RegExpExecArray | null;
    WIKI_LINK_RE.lastIndex = 0;
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
