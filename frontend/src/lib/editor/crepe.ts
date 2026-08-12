// Construction de l'éditeur Crepe (Milkdown) utilisé par NoteEditor.svelte.
//
// Crepe fournit l'UI « Notion-like » (slash menu, poignée de bloc, barre
// flottante de sélection, blocs image/tableau/code). Ce module y greffe ce
// qui est propre à NoteVault :
//   - la sérialisation Markdown non destructive pour les wiki-links,
//   - les décorations et l'autocomplétion `[[...]]`,
//   - la résolution des assets du coffre et le blocage des images distantes,
//   - le collage Markdown prioritaire et le drop de fichiers.
//
// La feature `AI` de Crepe n'est jamais activée : NoteVault ne fait aucun
// appel réseau depuis l'éditeur. La feature `Latex` et la barre supérieure
// sont hors périmètre produit.

import { syntaxHighlighting } from '@codemirror/language';
import { languages } from '@codemirror/language-data';
import { Prec } from '@codemirror/state';
import { classHighlighter } from '@lezer/highlight';
import { CrepeBuilder } from '@milkdown/crepe/builder';
import { blockEdit } from '@milkdown/crepe/feature/block-edit';
import { codeMirror } from '@milkdown/crepe/feature/code-mirror';
import { cursor } from '@milkdown/crepe/feature/cursor';
import { imageBlock } from '@milkdown/crepe/feature/image-block';
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip';
import { listItem } from '@milkdown/crepe/feature/list-item';
import { placeholder } from '@milkdown/crepe/feature/placeholder';
import { table } from '@milkdown/crepe/feature/table';
import { toolbar } from '@milkdown/crepe/feature/toolbar';
import {
  editorViewCtx,
  editorViewOptionsCtx,
  prosePluginsCtx,
  remarkStringifyOptionsCtx
} from '@milkdown/kit/core';
import { remarkGFMPlugin } from '@milkdown/kit/preset/gfm';
import { insert, replaceAll } from '@milkdown/kit/utils';
import { Plugin, PluginKey } from '@milkdown/kit/prose/state';
import type { Ctx } from '@milkdown/kit/ctx';
import type { EditorView } from '@milkdown/kit/prose/view';

import { BLOCKED_IMAGE_SRC, isLocalAssetPath, isSafeEditorImageSource, withTimeout } from '../assets';
import { plaintextMarkdownFromClipboard } from '../markdown-paste';
import { refreshWikiLinkDecorations, wikiLinkPlugin } from '../wiki-link';
import { wikiLinkSuggestionPlugin } from '../wiki-link-suggestion';
import { MARKDOWN_STRINGIFY_OPTIONS, preserveWikiLinksTextHandler } from './wiki-link-escape';

const ASSET_URL_TIMEOUT_MS = 5_000;

export type NoteEditorOptions = {
  root: HTMLElement;
  markdown: string;
  /** Titres de notes existantes, relu à chaque résolution (source vivante). */
  knownTitles: () => Set<string>;
  onWikiNavigate: (target: string) => void;
  onWikiCreate: (target: string) => void;
  /** Enregistre un fichier dans le coffre et retourne son chemin relatif. */
  onAssetUpload: (file: File) => Promise<string | null>;
  /** Résout un chemin relatif du coffre en URL du serveur d'assets local. */
  assetURL: (relativePath: string) => Promise<string>;
  /**
   * Appelé à chaque transaction modifiant le document, sans sérialisation :
   * l'hôte décide quand appeler `getMarkdown()` (débounce, flush avant save).
   */
  onDocChanged: () => void;
  /** Drop d'une URI (`file://`, `http(s)://`) ; retourne true si consommé. */
  onDropURI?: (uri: string) => boolean;
  placeholderText?: string;
};

export type NoteEditorHandle = {
  getMarkdown: () => string;
  /** Insère une image du coffre à la position courante (chemin relatif). */
  insertAsset: (relativePath: string, alt: string) => void;
  /** Remplace tout le contenu (restauration d'historique, recovery). */
  replaceMarkdown: (markdown: string) => void;
  /** Recalcule les décorations wiki-link après un changement de titres connus. */
  refreshWikiLinks: () => void;
  view: () => EditorView | null;
  destroy: () => Promise<void>;
};

// Les images ne sont chargées que depuis le coffre : le `src` du DOM passe
// par le serveur d'assets loopback, tout le reste tombe sur un placeholder.
// `proxyDomURL` n'affecte que le rendu — `node.attrs.src` garde le chemin
// relatif, donc le Markdown écrit sur disque reste inchangé.
function createProxyDomURL(assetURL: NoteEditorOptions['assetURL']) {
  return async (url: string): Promise<string> => {
    if (!isLocalAssetPath(url)) {
      // Déjà une URL du serveur d'assets local (rare) : on la laisse passer.
      return isSafeEditorImageSource(url) ? url : BLOCKED_IMAGE_SRC;
    }
    try {
      return await withTimeout(assetURL(url), ASSET_URL_TIMEOUT_MS, 'asset-url-timeout');
    } catch (error) {
      console.error('[editor] asset URL resolution failed:', error);
      return BLOCKED_IMAGE_SRC;
    }
  };
}

export async function createNoteEditor(options: NoteEditorOptions): Promise<NoteEditorHandle> {
  const crepe = new CrepeBuilder({ root: options.root, defaultValue: options.markdown });

  crepe.editor
    .config((ctx) => {
      // Options de sérialisation + handler `text` qui préserve `[[...]]`.
      ctx.update(remarkStringifyOptionsCtx, (prev) => ({
        ...prev,
        ...MARKDOWN_STRINGIFY_OPTIONS,
        handlers: { ...prev.handlers, text: preserveWikiLinksTextHandler }
      }));

      // Sans ça, remark aligne les tuyaux des tableaux sur la largeur des
      // cellules et réécrit toutes les lignes de séparation au premier save.
      ctx.set(remarkGFMPlugin.options.key, { tablePipeAlign: false });

      ctx.update(editorViewOptionsCtx, (prev) => ({
        ...prev,
        attributes: { class: 'note-editor-content', spellcheck: 'true' },
        handlePaste: (_view, event) => handleMarkdownPaste(ctx, event),
        handleDrop: (_view, event) => handleTextDrop(ctx, event, options.onDropURI)
      }));

      ctx.update(prosePluginsCtx, (plugins) => [
        ...plugins,
        docChangedPlugin(options.onDocChanged),
        wikiLinkPlugin({
          onNavigate: options.onWikiNavigate,
          onCreate: options.onWikiCreate,
          resolve: () => {
            const titles = options.knownTitles();
            return (target: string) => titles.has(target);
          }
        }),
        wikiLinkSuggestionPlugin({
          knownTitles: () => [...options.knownTitles()]
        })
      ]);
    });

  crepe
    .addFeature(cursor)
    .addFeature(listItem)
    .addFeature(linkTooltip)
    .addFeature(imageBlock, {
      onUpload: async (file: File) => (await options.onAssetUpload(file)) ?? '',
      proxyDomURL: createProxyDomURL(options.assetURL)
    })
    .addFeature(placeholder, { text: options.placeholderText ?? 'Écrire…' })
    .addFeature(blockEdit)
    .addFeature(toolbar)
    // `classHighlighter` émet des classes `tok-*` au lieu des couleurs
    // inline de `defaultHighlightStyle` : la coloration suit ainsi le thème
    // du coffre via le CSS (cf. styles.css), sans reconfigurer CodeMirror.
    .addFeature(codeMirror, {
      languages,
      extensions: [Prec.highest(syntaxHighlighting(classHighlighter))]
    })
    .addFeature(table);

  await crepe.create();

  const view = (): EditorView | null => {
    try {
      return crepe.editor.ctx.get(editorViewCtx);
    } catch {
      return null;
    }
  };

  return {
    getMarkdown: () => crepe.getMarkdown(),
    insertAsset: (relativePath: string, alt: string) => {
      crepe.editor.action(insert(`![${alt}](${relativePath})`));
    },
    replaceMarkdown: (markdown: string) => {
      crepe.editor.action(replaceAll(markdown));
    },
    refreshWikiLinks: () => refreshWikiLinkDecorations(view()),
    view,
    destroy: async () => {
      await crepe.destroy();
    }
  };
}

// Le Markdown structuré en text/plain a priorité, même si la source fournit
// aussi un flavor HTML. Un vrai collage riche sans syntaxe Markdown reste
// géré par ProseMirror. Tableaux GFM, tâches, titres et blocs de code
// deviennent ainsi immédiatement des nœuds riches au lieu de texte littéral.
function handleMarkdownPaste(ctx: Ctx, event: ClipboardEvent): boolean {
  const markdown = plaintextMarkdownFromClipboard(event.clipboardData);
  if (markdown === null) return false;
  event.preventDefault();
  insert(markdown)(ctx);
  return true;
}

// Drop d'URI ou de texte brut (wiki-link, snippet). Les fichiers image sont
// pris en charge en amont par le plugin `upload` de Crepe, câblé sur
// `onUpload` ; les URI (`file://` depuis un explorateur, `http(s)://` depuis
// un navigateur) sont déléguées à l'hôte, qui décide d'importer ou de
// refuser.
function handleTextDrop(
  ctx: Ctx,
  event: DragEvent,
  onDropURI: NoteEditorOptions['onDropURI']
): boolean {
  const files = event.dataTransfer?.files;
  if (files && files.length > 0) return false;

  const uri = event.dataTransfer?.getData('text/uri-list')?.trim();
  if (uri && onDropURI) {
    event.preventDefault();
    return onDropURI(uri);
  }

  const text = event.dataTransfer?.getData('text/plain');
  if (!text) return false;
  event.preventDefault();
  insert(text)(ctx);
  return true;
}

// Signal de modification immédiat, sans sérialisation Markdown : le plugin
// `listener` de Milkdown sérialise le document à chaque salve de frappe, ce
// qui est inutile ici puisque l'hôte débounce déjà et resérialise au flush.
function docChangedPlugin(onDocChanged: () => void): Plugin {
  return new Plugin({
    key: new PluginKey('notevault-doc-changed'),
    view: () => ({
      update: (view, prevState) => {
        if (!view.state.doc.eq(prevState.doc)) onDocChanged();
      }
    })
  });
}
