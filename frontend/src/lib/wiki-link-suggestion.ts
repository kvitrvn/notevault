// Plugin ProseMirror qui affiche une popup de suggestions quand l'utilisateur
// tape `[[` dans l'éditeur. La popup liste les titres de notes connus,
// filtrés en live au fur et à mesure de la frappe.
//
// Implémentation : DOM HTML direct (pas de mount Svelte). Plus simple, plus
// fiable, pas de fuite mémoire potentielle. Le positionnement et le cycle
// affichage/masquage sont délégués à `SlashProvider` (floating-ui), qui
// ancre la popup sous le curseur ; le clavier reste géré ici via
// `handleKeyDown`.
//
// Roundtrip Markdown : la sélection insère `[[Titre]]` en clair. Les
// décorations wiki-link de lib/wiki-link.ts s'appliquent ensuite pour la
// surbrillance et la navigation au click.

import { SlashProvider } from '@milkdown/kit/plugin/slash';
import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import type { EditorView } from '@milkdown/kit/prose/view';

export type WikiLinkSuggestionOptions = {
  knownTitles: () => string[];
};

/** Position du `[[` courant dans le document, bornes incluses. */
export type WikiLinkRange = { from: number; to: number };

const PLUGIN_KEY = new PluginKey('wikiLinkSuggestion');
const MAX_ITEMS = 8;
/** Au-delà, ce n'est plus une recherche de titre mais du texte qui contient `[[`. */
const MAX_QUERY_LENGTH = 120;

export function shouldShowWikiLinkSuggestion(query: string): boolean {
  return !query.includes(']]');
}

/**
 * Extrait la requête en cours à partir du texte qui précède le curseur dans
 * le bloc courant. Retourne null s'il n'y a pas de `[[` ouvert.
 */
export function findWikiLinkQuery(textBefore: string): string | null {
  const start = textBefore.lastIndexOf('[[');
  if (start === -1) return null;
  const query = textBefore.slice(start + 2);
  if (query.length > MAX_QUERY_LENGTH) return null;
  if (query.includes('\n')) return null;
  if (!shouldShowWikiLinkSuggestion(query)) return null;
  return query;
}

/** Titres commençant par la requête d'abord, puis ceux qui la contiennent. */
export function rankWikiLinkTitles(titles: string[], query: string): string[] {
  const q = query.toLowerCase();
  if (!q) return titles.slice(0, MAX_ITEMS);

  const starts: string[] = [];
  const contains: string[] = [];
  for (const title of titles) {
    const lower = title.toLowerCase();
    if (lower.startsWith(q)) {
      starts.push(title);
    } else if (lower.includes(q)) {
      contains.push(title);
    }
  }
  return [...starts, ...contains].slice(0, MAX_ITEMS);
}

/** Remplace le `[[requête` en cours par `[[Titre]]` et replace le curseur après. */
export function completeWikiLinkSuggestion(
  view: EditorView,
  range: WikiLinkRange,
  title: string
): void {
  const text = `[[${title}]]`;
  const tr = view.state.tr.insertText(text, range.from, range.to);
  view.dispatch(tr);
  view.focus();
}

export function wikiLinkSuggestionPlugin(opts: WikiLinkSuggestionOptions): Plugin {
  // `handleKeyDown` vit dans `props` et n'a pas accès au PluginView : on garde
  // donc l'instance courante dans la closure du plugin.
  let instance: WikiLinkSuggestionView | null = null;

  return new Plugin({
    key: PLUGIN_KEY,
    view: (view) => {
      instance = new WikiLinkSuggestionView(view, opts);
      return {
        update: (updatedView, prevState) => instance?.update(updatedView, prevState),
        destroy: () => {
          instance?.destroy();
          instance = null;
        }
      };
    },
    props: {
      handleKeyDown: (_view, event) => instance?.handleKeyDown(event) ?? false
    }
  });
}

class WikiLinkSuggestionView {
  readonly #provider: SlashProvider;
  readonly #host: HTMLDivElement;
  readonly #list: HTMLDivElement;
  readonly #opts: WikiLinkSuggestionOptions;
  #view: EditorView;
  #items: string[] = [];
  #selectedIndex = 0;
  #range: WikiLinkRange | null = null;
  #visible = false;

  constructor(view: EditorView, opts: WikiLinkSuggestionOptions) {
    this.#view = view;
    this.#opts = opts;

    this.#host = document.createElement('div');
    this.#host.className = 'wiki-link-popup-host';

    this.#list = document.createElement('div');
    this.#list.className = 'wiki-link-popup';
    this.#list.setAttribute('role', 'listbox');
    this.#list.setAttribute('aria-label', 'Suggestions de wiki-lien');
    this.#host.appendChild(this.#list);

    this.#provider = new SlashProvider({
      content: this.#host,
      debounce: 0,
      offset: 4,
      shouldShow: (activeView) => this.#refreshQuery(activeView)
    });
    this.#provider.onShow = () => {
      this.#visible = true;
    };
    this.#provider.onHide = () => {
      this.#visible = false;
      this.#range = null;
    };

    this.#provider.update(view);
  }

  update(view: EditorView, prevState?: Parameters<SlashProvider['update']>[1]): void {
    this.#view = view;
    this.#provider.update(view, prevState);
  }

  destroy(): void {
    this.#provider.destroy();
    this.#host.remove();
  }

  handleKeyDown(event: KeyboardEvent): boolean {
    if (!this.#visible) return false;

    if (event.key === 'Escape') {
      this.#provider.hide();
      event.preventDefault();
      return true;
    }
    if (event.key === 'ArrowDown') {
      this.#move(1);
      event.preventDefault();
      return true;
    }
    if (event.key === 'ArrowUp') {
      this.#move(-1);
      event.preventDefault();
      return true;
    }
    if (event.key === 'Enter' || event.key === 'Tab') {
      const item = this.#items[this.#selectedIndex];
      if (item && this.#range) {
        this.#complete(item);
      }
      event.preventDefault();
      return true;
    }
    return false;
  }

  // Recalcule requête, items et bornes ; retourne false pour masquer la popup.
  #refreshQuery(view: EditorView): boolean {
    const content = this.#provider.getContent(view);
    if (content === undefined) return false;

    const query = findWikiLinkQuery(content);
    if (query === null) return false;

    const cursor = view.state.selection.from;
    this.#range = { from: cursor - query.length - 2, to: cursor };
    this.#items = rankWikiLinkTitles(this.#opts.knownTitles(), query);
    this.#selectedIndex = 0;
    this.#renderList();
    return true;
  }

  #move(delta: number): void {
    const count = this.#items.length;
    if (count === 0) return;
    this.#selectedIndex = (this.#selectedIndex + delta + count) % count;
    this.#renderList();
  }

  #complete(title: string): void {
    if (!this.#range) return;
    const range = this.#range;
    this.#provider.hide();
    completeWikiLinkSuggestion(this.#view, range, title);
  }

  #renderList(): void {
    this.#list.replaceChildren();

    if (this.#items.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'wiki-link-popup__empty';
      empty.textContent = 'Aucune note';
      this.#list.appendChild(empty);
      return;
    }

    this.#items.forEach((item, index) => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'wiki-link-popup__item';
      if (index === this.#selectedIndex) button.classList.add('is-active');
      button.setAttribute('role', 'option');
      button.setAttribute('aria-selected', String(index === this.#selectedIndex));
      button.textContent = item;
      button.addEventListener('mousedown', (event) => {
        event.preventDefault();
        this.#complete(item);
      });
      button.addEventListener('mouseenter', () => {
        this.#selectedIndex = index;
        this.#renderList();
      });
      this.#list.appendChild(button);
    });
  }
}
