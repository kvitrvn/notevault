// Extension Tiptap qui affiche une popup de suggestions quand l'utilisateur
// tape `[[` dans l'éditeur. La popup liste les titres de notes connus,
// filtrés en live au fur et à mesure de la frappe.
//
// Implémentation : DOM HTML direct (pas de mount Svelte). Plus simple, plus
// fiable, pas de fuite mémoire potentielle. Le popup est un div positionné
// en `fixed` au-dessus de l'éditeur, démonté à chaque sortie de Suggestion.
//
// Roundtrip Markdown : la sélection insère `[[Titre]]` en clair. Les
// décorations wiki-link de lib/wiki-link.ts s'appliquent ensuite pour la
// surbrillance et la navigation au click.

import { Extension, type Editor } from '@tiptap/core';
import Suggestion, { exitSuggestion, type SuggestionOptions } from '@tiptap/suggestion';
import { PluginKey } from '@tiptap/pm/state';
import type { Range } from '@tiptap/core';

export type WikiLinkSuggestionOptions = {
  knownTitles: () => string[];
};

const PLUGIN_KEY = new PluginKey('wikiLinkSuggestion');

export function shouldShowWikiLinkSuggestion(query: string): boolean {
  return !query.includes(']]');
}

export function completeWikiLinkSuggestion(editor: Editor, range: Range, title: string): void {
  editor.chain().focus().insertContentAt(range, `[[${title}]]`).run();
  // Le matcher autorise les espaces et considère sinon les `]]` comme une
  // partie de la requête. Sans sortie explicite, la popup reste active et
  // capture Entrée/Tab après la sélection du lien.
  exitSuggestion(editor.view, PLUGIN_KEY);
}

export const WikiLinkSuggestion = Extension.create<WikiLinkSuggestionOptions>({
  name: 'wikiLinkSuggestion',

  addOptions() {
    return {
      knownTitles: () => []
    };
  },

  addProseMirrorPlugins() {
    const opts = this.options;
    const editor: Editor = this.editor;

    const suggestionOptions: Omit<SuggestionOptions<string>, 'editor'> = {
      char: '[[',
      startOfLine: false,
      allowSpaces: true,
      allowedPrefixes: null,
      pluginKey: PLUGIN_KEY,
      shouldShow: ({ query }) => shouldShowWikiLinkSuggestion(query),
      items: ({ query }) => {
        const all = opts.knownTitles();
        const q = query.toLowerCase();
        if (!q) return all.slice(0, 8);
        const starts: string[] = [];
        const contains: string[] = [];
        for (const t of all) {
          const lower = t.toLowerCase();
          if (lower.startsWith(q)) {
            starts.push(t);
          } else if (lower.includes(q)) {
            contains.push(t);
          }
        }
        return [...starts, ...contains].slice(0, 8);
      },
      command: ({ editor: ed, range, props }) => {
        completeWikiLinkSuggestion(ed, range, props);
      },
      render: () => {
        let host: HTMLDivElement | null = null;
        let listEl: HTMLDivElement | null = null;
        let items: string[] = [];
        let selectedIndex = 0;
        let commandFn: ((item: string) => void) | null = null;
        let onDocPointerDown: ((event: MouseEvent) => void) | null = null;

        const updatePosition = (clientRect: DOMRect | null) => {
          if (!host || !clientRect) return;
          host.style.left = `${clientRect.left}px`;
          host.style.top = `${clientRect.bottom + 4}px`;
        };

        const renderList = () => {
          if (!listEl) return;
          listEl.innerHTML = '';
          if (items.length === 0) {
            const empty = document.createElement('div');
            empty.className = 'wiki-link-popup__empty';
            empty.textContent = 'Aucune note';
            listEl.appendChild(empty);
            return;
          }
          items.forEach((item, index) => {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'wiki-link-popup__item';
            if (index === selectedIndex) btn.classList.add('is-active');
            btn.setAttribute('role', 'option');
            btn.setAttribute('aria-selected', String(index === selectedIndex));
            btn.textContent = item;
            btn.addEventListener('mousedown', (e) => {
              e.preventDefault();
              commandFn?.(item);
            });
            btn.addEventListener('mouseenter', () => {
              selectedIndex = index;
              renderList();
            });
            listEl!.appendChild(btn);
          });
        };

        const destroy = () => {
          if (onDocPointerDown) {
            document.removeEventListener('mousedown', onDocPointerDown, true);
            onDocPointerDown = null;
          }
          if (host) {
            host.remove();
            host = null;
            listEl = null;
          }
          items = [];
          selectedIndex = 0;
          commandFn = null;
        };

        return {
          onStart: (props) => {
            items = props.items;
            // Tiptap suggestion's `props.command(item)` attend l'item
            // directement : il s'occupe déjà de wrapper dans
            // `{ editor, range, props }` côté interne.
            commandFn = (item) => props.command(item);
            selectedIndex = 0;

            host = document.createElement('div');
            host.className = 'wiki-link-popup-host';

            listEl = document.createElement('div');
            listEl.className = 'wiki-link-popup';
            listEl.setAttribute('role', 'listbox');
            listEl.setAttribute('aria-label', 'Suggestions de wiki-lien');

            host.appendChild(listEl);
            document.body.appendChild(host);

            renderList();
            updatePosition(props.clientRect?.() ?? null);

            onDocPointerDown = (event) => {
              if (!host) return;
              if (event.target instanceof Node && host.contains(event.target)) return;
              editor.commands.focus();
              destroy();
            };
            document.addEventListener('mousedown', onDocPointerDown, true);
          },

          onUpdate: (props) => {
            items = props.items;
            selectedIndex = 0;
            renderList();
            updatePosition(props.clientRect?.() ?? null);
          },

          onKeyDown: (props) => {
            if (props.event.key === 'Escape') {
              destroy();
              return true;
            }
            if (props.event.key === 'ArrowDown') {
              selectedIndex = (selectedIndex + 1) % Math.max(items.length, 1);
              renderList();
              props.event.preventDefault();
              return true;
            }
            if (props.event.key === 'ArrowUp') {
              selectedIndex = (selectedIndex - 1 + items.length) % Math.max(items.length, 1);
              renderList();
              props.event.preventDefault();
              return true;
            }
            if (props.event.key === 'Enter' || props.event.key === 'Tab') {
              if (items.length > 0 && commandFn) {
                commandFn(items[selectedIndex]);
              }
              props.event.preventDefault();
              return true;
            }
            return false;
          },

          onExit: () => {
            destroy();
          }
        };
      }
    };

    return [
      Suggestion({
        editor,
        ...suggestionOptions
      })
    ];
  }
});
