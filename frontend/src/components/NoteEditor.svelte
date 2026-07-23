<script lang="ts">
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import Columns3 from '@lucide/svelte/icons/columns-3';
  import Rows3 from '@lucide/svelte/icons/rows-3';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { onDestroy, onMount, untrack } from 'svelte';
  import { Editor, mergeAttributes } from '@tiptap/core';
  import StarterKit from '@tiptap/starter-kit';
  import Image from '@tiptap/extension-image';
  import CodeBlockLowlight from '@tiptap/extension-code-block-lowlight';
  import { common, createLowlight } from 'lowlight';
  import { Markdown } from '@tiptap/markdown';
  import { TableKit } from '@tiptap/extension-table';
  import { TaskItem, TaskList } from '@tiptap/extension-list';
  import { WikiLink, refreshWikiLinkDecorations } from '../lib/wiki-link';
  import { WikiLinkSuggestion } from '../lib/wiki-link-suggestion';
  import EditorToolbar, { type BlockValue, type ToolbarState } from './EditorToolbar.svelte';
  import {
    isRemoteImageSource,
    isSafeEditorImageSource,
    scrubAbsoluteAssetURLs,
    withTimeout
  } from '../lib/assets';
  import { plaintextMarkdownFromClipboard } from '../lib/markdown-paste';

  type Props = {
    markdown?: string;
    onChange?: (value: string) => void;
    onDirty?: () => void;
    knownTitles?: Set<string>;
    onWikiNavigate?: (target: string) => void;
    onWikiCreate?: (target: string) => void;
    onAssetUpload?: (file: File) => Promise<string | null>;
    onAssetImportFromPath?: (absolutePath: string) => Promise<string | null>;
    assetURL?: (relPath: string) => Promise<string>;
    onReady?: (state: { isEditable: boolean; isFocused: boolean }) => void;
    onError?: (error: unknown) => void;
  };

  let {
    markdown = '',
    onChange = () => {},
    onDirty = () => {},
    knownTitles = new Set(),
    onWikiNavigate = () => {},
    onWikiCreate = () => {},
    onAssetUpload = async () => null,
    onAssetImportFromPath = async () => null,
    assetURL = async (rel) => rel,
    onReady = () => {},
    onError = () => {}
  }: Props = $props();

  let host: HTMLDivElement | undefined = $state();
  // `$state.raw` : seule la réassignation (null → instance) est réactive.
  // Surtout pas `$state` — le proxy profond casserait ProseMirror. Les
  // changements internes à l'instance sont signalés par `editorVersion`.
  let editor: Editor | null = $state.raw(null);
  let lastMarkdown = markdown;
  // Suit la dernière valeur de la prop `markdown` que NoteEditor a vue.
  // Sert à distinguer un vrai changement externe (autre note ouverte,
  // restore historique, recovery) d'une simple réassignation Svelte avec
  // contenu byte-égal (ex. `selected = saved` dans `flushSave`). Sans ce
  // tracker, le bloc réactif `markdown !== lastMarkdown` se déclenchait
  // sporadiquement pendant le cycle save et dispatchait un `replaceWith`
  // complet qui démontait/remontait les décorations wiki-link → flicker.
  let lastSeenPropMarkdown = markdown;
  let editorReady = $state(false);
  let editorEditable = $state(false);
  // Tick incrémenté à chaque transaction ProseMirror : c'est le signal
  // d'invalidation du snapshot `toolbar` ($derived). L'instance `editor`
  // étant en $state.raw, ses mutations internes sont invisibles pour
  // Svelte — ce compteur primitif force la ré-évaluation.
  let editorVersion = $state(0);
  let pendingChangeTimer: ReturnType<typeof setTimeout> | null = null;
  let hasPendingChange = false;

  // Menu flottant ancré au curseur, visible uniquement quand le curseur
  // est dans une cellule de tableau. Positionné en `fixed` à partir des
  // coords écran fournies par `editor.view.coordsAtPos`.
  let tableMenuVisible = $state(false);
  let tableMenuX = $state(0);
  let tableMenuY = $state(0);
  let tableMenuPlacement: 'top' | 'bottom' = $state('top');
  const TABLE_MENU_HEIGHT = 36;

  const lowlight = createLowlight(common);
  const MARKDOWN_CHANGE_DEBOUNCE_MS = 200;
  const isDev = Boolean((import.meta as ImportMeta & { env?: { DEV?: boolean } }).env?.DEV);

  // Le Markdown conserve la source distante, mais aucun élément <img> n'est
  // créé pour elle : ouvrir une note ne doit pas contacter un serveur tiers.
  const VaultImage = Image.extend({
    renderHTML({ HTMLAttributes }) {
      if (!isSafeEditorImageSource(HTMLAttributes.src)) {
        return [
          'span',
          mergeAttributes(this.options.HTMLAttributes, {
            class: 'note-editor-blocked-image',
            'data-blocked-image': 'true'
          }),
          'Image distante bloquée'
        ];
      }
      return ['img', mergeAttributes(this.options.HTMLAttributes, HTMLAttributes)];
    }
  });

  // Toute transaction ProseMirror (saisie, sélection, meta) passe par
  // `onTransaction`, donc on centralise la notification ici.
  function notifyEditorChange(): void {
    editorVersion++;
  }

  const DISABLED_ITEM = { active: false, enabled: false };
  const DISABLED_TOOLBAR_STATE: ToolbarState = {
    canUndo: false,
    canRedo: false,
    block: 'paragraph',
    blockEnabled: false,
    bold: DISABLED_ITEM,
    italic: DISABLED_ITEM,
    strike: DISABLED_ITEM,
    bulletList: DISABLED_ITEM,
    orderedList: DISABLED_ITEM,
    taskList: DISABLED_ITEM,
    blockquote: DISABLED_ITEM,
    codeBlock: DISABLED_ITEM,
    canInsertTable: false,
    canInsertHorizontalRule: false
  };

  // Snapshot de l'état éditeur consommé par <EditorToolbar>. `editorVersion`
  // est lu en premier et inconditionnellement : c'est lui qui invalide le
  // snapshot à chaque transaction, y compris tant que `editor` est null.
  // `enabled` vient de `can()` : le schéma interdit certaines commandes
  // selon le contexte (citation dans un task item, gras dans un code
  // block, etc.) — le bouton correspondant est grisé plutôt qu'inerte.
  const toolbar: ToolbarState = $derived.by(() => {
    editorVersion;
    const ed = editor;
    if (!ed || ed.isDestroyed) return DISABLED_TOOLBAR_STATE;

    const can = ed.can();
    const block: BlockValue = ed.isActive('heading', { level: 1 })
      ? 'heading-1'
      : ed.isActive('heading', { level: 2 })
        ? 'heading-2'
        : ed.isActive('heading', { level: 3 })
          ? 'heading-3'
          : 'paragraph';

    return {
      canUndo: can.undo(),
      canRedo: can.redo(),
      block,
      blockEnabled: can.setParagraph() || can.toggleHeading({ level: 1 }),
      bold: { active: ed.isActive('bold'), enabled: can.toggleBold() },
      italic: { active: ed.isActive('italic'), enabled: can.toggleItalic() },
      strike: { active: ed.isActive('strike'), enabled: can.toggleStrike() },
      bulletList: { active: ed.isActive('bulletList'), enabled: can.toggleBulletList() },
      orderedList: { active: ed.isActive('orderedList'), enabled: can.toggleOrderedList() },
      taskList: { active: ed.isActive('taskList'), enabled: can.toggleTaskList() },
      blockquote: { active: ed.isActive('blockquote'), enabled: can.toggleBlockquote() },
      codeBlock: { active: ed.isActive('codeBlock'), enabled: can.toggleCodeBlock() },
      canInsertTable: can.insertTable({ rows: 3, cols: 3, withHeaderRow: true }),
      canInsertHorizontalRule: can.setHorizontalRule()
    };
  });

  // Recalcule la position du menu flottant. Appelée à chaque transaction
  // (sélection, frappe, etc.). Le menu se place au-dessus du curseur par
  // défaut, et bascule en dessous s'il n'y a pas la place.
  function updateTableMenu(): void {
    if (!editor || !editor.isActive('table')) {
      tableMenuVisible = false;
      return;
    }
    const { from } = editor.state.selection;
    const coords = editor.view.coordsAtPos(from);
    const spaceAbove = coords.top;
    tableMenuPlacement = spaceAbove > TABLE_MENU_HEIGHT + 12 ? 'top' : 'bottom';
    tableMenuX = coords.left;
    tableMenuY =
      tableMenuPlacement === 'top' ? coords.top - TABLE_MENU_HEIGHT - 6 : coords.bottom + 6;
    tableMenuVisible = true;
  }

  function hideTableMenu(): void {
    tableMenuVisible = false;
  }

  onMount(() => {
    if (!host) return;
    try {
      editor = new Editor({
        element: host,
        editable: true,
        extensions: [
          StarterKit.configure({
            codeBlock: false
          }),
          CodeBlockLowlight.configure({ lowlight }),
          VaultImage.configure({
            inline: false,
            allowBase64: false
          }),
          TableKit,
          TaskList,
          TaskItem.configure({
            nested: true
          }),
          Markdown,
          WikiLink.configure({
            onNavigate: (t) => onWikiNavigate(t),
            onCreate: (t) => onWikiCreate(t),
            resolve: () => (target: string) => knownTitles.has(target)
          }),
          WikiLinkSuggestion.configure({
            knownTitles: () => [...knownTitles]
          })
        ],
        content: markdown,
        contentType: 'markdown',
        editorProps: {
          attributes: {
            class: 'note-editor-content',
            spellcheck: 'true'
          },
          handlePaste: (_view, event) => {
            const items = Array.from(event.clipboardData?.items ?? []);
            const imageItem = items.find((it) => it.type.startsWith('image/'));
            if (imageItem) {
              const file = imageItem.getAsFile();
              if (!file) return false;
              event.preventDefault();
              void handleAssetInsert(file);
              return true;
            }

            // Le Markdown structuré en text/plain a priorité, même si la
            // source fournit aussi un flavor HTML. Un vrai collage riche sans
            // syntaxe Markdown reste géré par ProseMirror. Tableaux GFM,
            // tâches, titres et blocs de code deviennent ainsi immédiatement
            // des nœuds riches au lieu de texte littéral.
            const markdownText = plaintextMarkdownFromClipboard(event.clipboardData);
            if (markdownText === null || !editor) return false;
            event.preventDefault();
            return editor
              .chain()
              .focus()
              .insertContent(markdownText, { contentType: 'markdown' })
              .run();
          },
          handleDrop: (view, event) => {
            if (!event.dataTransfer) return false;

            // 1) Fichier image (drop depuis filesystem ou explorateur).
            //    On accepte aussi les fichiers sans MIME déclaré.
            const IMAGE_EXTS = /\.(png|jpe?g|gif|webp|svg|bmp|ico|avif)$/i;
            const file = Array.from(event.dataTransfer.files).find((f) =>
              f.type.startsWith('image/') || IMAGE_EXTS.test(f.name)
            );
            if (file) {
              event.preventDefault();
              void handleAssetInsert(file);
              return true;
            }

            // 2) URL d'image (drop depuis un onglet browser, signet, extension).
            //    Si text/uri-list contient une URL, on l'utilise directement.
            //    Sinon, on tente d'extraire un <img src="..."> du text/html.
            const items = Array.from(event.dataTransfer.items ?? []);
            const urlItem = items.find((it) => it.kind === 'string' && it.type === 'text/uri-list');
            const htmlItem = items.find((it) => it.kind === 'string' && it.type === 'text/html');
            if (urlItem || htmlItem) {
              event.preventDefault();
              urlItem?.getAsString((uri) => {
                const trimmed = uri.trim();
                if (trimmed) {
                  void handleRemoteImage(trimmed);
                  return;
                }
                htmlItem?.getAsString((html) => {
                  const match = html.match(/<img[^>]+src=["']([^"']+)["']/i);
                  if (match && match[1]) {
                    void handleRemoteImage(match[1]);
                  }
                });
              });
              return true;
            }

            // 3) Texte plain : on l'insère tel quel (wikilink, snippet, etc.).
            const textItem = items.find((it) => it.kind === 'string' && it.type === 'text/plain');
            if (textItem) {
              event.preventDefault();
              textItem.getAsString((text) => {
                if (text) editor?.chain().focus().insertContent(text).run();
              });
              return true;
            }

            return false;
          }
        },
        onCreate: ({ editor: ed }) => {
          // Au load, le doc contient déjà des `src` absolus (pré-transformés
          // par App.svelte via assetURL). On installe juste le scrubber pour
          // remettre les chemins relatifs au save.
          installMarkdownScrubber(ed);
          editorReady = true;
          editorEditable = ed.isEditable;
          onReady({ isEditable: ed.isEditable, isFocused: ed.isFocused });
        },
        onTransaction: () => {
          updateTableMenu();
          notifyEditorChange();
        },
        onBlur: () => {
          hideTableMenu();
        },
        onUpdate: ({ editor: updatedEditor }) => {
          onDirty();
          schedulePendingChange(updatedEditor);
        }
      });
    } catch (err) {
      onError(err);
      console.error('[editor] init failed:', err);
    }
  });

  function installMarkdownScrubber(ed: Editor): void {
    const original = ed.getMarkdown.bind(ed);
    ed.getMarkdown = () => scrubAbsoluteAssetURLs(original());
  }

  function serializeMarkdown(ed: Editor): string {
    if (isDev) console.time('NoteEditor:getMarkdown');
    try {
      return scrubAbsoluteAssetURLs(ed.getMarkdown());
    } finally {
      if (isDev) console.timeEnd('NoteEditor:getMarkdown');
    }
  }

  function schedulePendingChange(ed: Editor): void {
    hasPendingChange = true;
    if (pendingChangeTimer) clearTimeout(pendingChangeTimer);
    pendingChangeTimer = setTimeout(() => {
      if (editor === ed) emitPendingChange();
    }, MARKDOWN_CHANGE_DEBOUNCE_MS);
  }

  function emitPendingChange(): void {
    if (!editor || !hasPendingChange) return;
    if (pendingChangeTimer) {
      clearTimeout(pendingChangeTimer);
      pendingChangeTimer = null;
    }
    hasPendingChange = false;
    const value = serializeMarkdown(editor);
    lastMarkdown = value;
    onChange(value);
  }

  export function flushPendingChange(): void {
    emitPendingChange();
  }

  let isUploading = $state(false);
  let uploadError = $state('');

  // Les fichiers locaux sont importés par Go. Les URL distantes sont refusées
  // pour préserver la promesse local-first et éviter les pixels de suivi.
  async function handleRemoteImage(uri: string): Promise<void> {
    if (!editor) return;
    isUploading = true;
    uploadError = '';
    try {
      // Cas particulier : file:// — on délègue au backend Go qui n'a pas
      // la restriction CORS. C'est le cas typique des drops depuis
      // Nautilus/Dolphin sur WebKit Linux.
      if (/^file:\/\//i.test(uri)) {
        const absPath = decodeURIComponent(uri.replace(/^file:\/\//i, ''));
        const relPath = await withTimeout(
          onAssetImportFromPath(absPath),
          15000,
          'Import timeout (15s)'
        );
        if (!relPath) return;
        const alt = absPath.split('/').pop()?.replace(/\?.*$/, '') || 'image';
        const absoluteURL = await withTimeout(
          assetURL(relPath),
          5000,
          'assetURL timeout (5s)'
        );
        editor.chain().focus().setImage({ src: absoluteURL, alt }).run();
        return;
      }
      uploadError = isRemoteImageSource(uri)
        ? 'Images distantes bloquées — téléchargez le fichier avant de l’ajouter'
        : 'Source d’image non locale bloquée';
      setTimeout(() => (uploadError = ''), 4000);
    } catch (err) {
      uploadError = `Image distante : ${err}`;
      setTimeout(() => (uploadError = ''), 4000);
    } finally {
      isUploading = false;
    }
  }

  async function handleAssetInsert(file: File): Promise<void> {
    if (!editor) return;
    if (file.size > 10 * 1024 * 1024) {
      uploadError = 'Image trop volumineuse (>10 MB)';
      setTimeout(() => (uploadError = ''), 4000);
      return;
    }
    isUploading = true;
    uploadError = '';
    try {
      const relPath = await withTimeout(
        onAssetUpload(file),
        15000,
        'Upload timeout (15s)'
      );
      if (!relPath) return;
      const alt = file.name.replace(/\.[^.]+$/, '');
      const absoluteURL = await withTimeout(
        assetURL(relPath),
        5000,
        'assetURL timeout (5s)'
      );
      editor.chain().focus().setImage({ src: absoluteURL, alt }).run();
    } catch (err) {
      uploadError = String(err);
      setTimeout(() => (uploadError = ''), 4000);
    } finally {
      isUploading = false;
    }
  }

  // Recharge le contenu de l'éditeur quand la prop `markdown` arrive de
  // l'extérieur (ouverture d'une autre note, restauration d'historique,
  // recovery buffer, etc.). Tiptap n'expose pas `addToHistory` sur
  // `setContent` ; on dispatch donc la transaction à la main avec le meta
  // `addToHistory: false` pour ne PAS empiler un "remplace tout le doc"
  // dans la pile d'undo à chaque sauvegarde côté backend. `preventUpdate`
  // évite la boucle avec le callback `onUpdate`.
  //
  // On compare `markdown` à `lastSeenPropMarkdown` (pas à `lastMarkdown`)
  // pour distinguer :
  //   - vrai changement externe → on recharge le doc ;
  //   - réassignation Svelte byte-égale (`selected = saved` après save) →
  //     on ne fait rien, sinon le `replaceWith` démonte les décorations
  //     wiki-link et provoque un flicker visuel du texte pendant ~100 ms.
  // Si la prop correspond déjà à l'état sérialisé de l'éditeur, on n'a
  // rien à faire non plus (cas du restore où la prop arrive identique).
  // Si la prop diffère au niveau string mais que le JSON parsé est
  // structurellement identique au doc courant, on aligne les références
  // sans dispatcher de transaction (évite un re-instantiate de décorations).
  //
  // Dépendances trackées : uniquement `markdown` et `editor` (capturés
  // AVANT untrack). Tout le corps est sous `untrack` parce que le dispatch
  // déclenche `onTransaction`, qui écrit `editorVersion` et les états du
  // menu tableau : si l'un d'eux devenait une dépendance, l'effet
  // bouclerait (effect_update_depth_exceeded). Les trackers `lastMarkdown`,
  // `lastSeenPropMarkdown`, `pendingChangeTimer` et `hasPendingChange`
  // restent des `let` non réactifs pour la même raison.
  $effect(() => {
    const md = markdown;
    const ed = editor;
    if (!ed) return;
    untrack(() => {
      if (md === lastSeenPropMarkdown) return;
      lastSeenPropMarkdown = md;
      if (md === lastMarkdown) return;
      if (pendingChangeTimer) {
        clearTimeout(pendingChangeTimer);
        pendingChangeTimer = null;
      }
      hasPendingChange = false;
      try {
        const json = ed.markdown?.parse(md) ?? { type: 'doc', content: [] };
        const nextDoc = ed.schema.nodeFromJSON(json);
        const currentJSON = ed.state.doc.toJSON();
        if (JSON.stringify(currentJSON) !== JSON.stringify(nextDoc.toJSON())) {
          ed.view.dispatch(
            ed.state.tr
              .replaceWith(0, ed.state.doc.content.size, nextDoc)
              .setMeta('addToHistory', false)
              .setMeta('preventUpdate', true)
          );
        }
      } catch (err) {
        console.error('[editor] setContent failed:', err);
      }
      lastMarkdown = md;
    });
  });

  // Rafraîchit les décorations wiki-link quand la liste des titres connus
  // change (création, suppression, ouverture d'une autre note). Seule
  // l'identité de la prop `knownTitles` est trackée.
  $effect(() => {
    knownTitles;
    const ed = editor;
    if (!ed) return;
    untrack(() => refreshWikiLinkDecorations(ed));
  });

  function undo(): void {
    editor?.chain().focus().undo().run();
  }

  function redo(): void {
    editor?.chain().focus().redo().run();
  }

  function toggleBold(): void {
    editor?.chain().focus().toggleBold().run();
  }

  function toggleItalic(): void {
    editor?.chain().focus().toggleItalic().run();
  }

  function toggleStrike(): void {
    editor?.chain().focus().toggleStrike().run();
  }

  function changeBlock(value: BlockValue): void {
    const chain = editor?.chain().focus();

    if (!chain) return;

    if (value === 'paragraph') {
      chain.setParagraph().run();
      return;
    }

    const level = Number(value.replace('heading-', '')) as 1 | 2 | 3;
    chain.toggleHeading({ level }).run();
  }

  function toggleBulletList(): void {
    editor?.chain().focus().toggleBulletList().run();
  }

  function toggleOrderedList(): void {
    editor?.chain().focus().toggleOrderedList().run();
  }

  function toggleTaskList(): void {
    editor?.chain().focus().toggleTaskList().run();
  }

  function toggleBlockquote(): void {
    editor?.chain().focus().toggleBlockquote().run();
  }

  // Bouton "Image" : ouvre un file picker, plus fiable que le drag&drop
  // qui est mal supporté par WebKit sur Linux pour les fichiers locaux.
  let fileInput: HTMLInputElement | null = $state(null);
  function openImagePicker(): void {
    fileInput?.click();
  }
  function onFilePickerChange(event: Event): void {
    const target = event.currentTarget as HTMLInputElement;
    const file = target.files?.[0];
    if (file) {
      void handleAssetInsert(file);
    }
    target.value = '';
  }

  function toggleCodeBlock(): void {
    editor?.chain().focus().toggleCodeBlock().run();
  }

  function insertTable(): void {
    editor
      ?.chain()
      .focus()
      .insertTable({
        rows: 3,
        cols: 3,
        withHeaderRow: true
      })
      .run();
  }

  function deleteTable(): void {
    editor?.chain().focus().deleteTable().run();
  }

  function addRowBefore(): void {
    editor?.chain().focus().addRowBefore().run();
  }

  function addRowAfter(): void {
    editor?.chain().focus().addRowAfter().run();
  }

  function addColumnBefore(): void {
    editor?.chain().focus().addColumnBefore().run();
  }

  function addColumnAfter(): void {
    editor?.chain().focus().addColumnAfter().run();
  }

  function deleteRow(): void {
    editor?.chain().focus().deleteRow().run();
  }

  function deleteColumn(): void {
    editor?.chain().focus().deleteColumn().run();
  }

  function insertHorizontalRule(): void {
    editor?.chain().focus().setHorizontalRule().run();
  }

  onDestroy(() => {
    if (pendingChangeTimer) {
      clearTimeout(pendingChangeTimer);
      pendingChangeTimer = null;
    }
    editor?.destroy();
  });

  // Drag-over visuel : on tracke les events dragenter/dragleave sur l'host
  // pour afficher un overlay "Déposez l'image ici".
  let dragOverCount = $state(0);
  function onHostDragEnter(event: DragEvent): void {
    if (!event.dataTransfer) return;
    const types = Array.from(event.dataTransfer.types ?? []);
    if (types.includes('Files')) {
      dragOverCount += 1;
      event.preventDefault();
    }
  }
  function onHostDragLeave(): void {
    dragOverCount = Math.max(0, dragOverCount - 1);
  }
  function onHostDragOver(event: DragEvent): void {
    const types = Array.from(event.dataTransfer?.types ?? []);
    if (types.includes('Files')) {
      event.preventDefault();
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
    }
  }
</script>

<div class="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background text-foreground">
  <EditorToolbar
    {toolbar}
    onUndo={undo}
    onRedo={redo}
    onChangeBlock={changeBlock}
    onToggleBold={toggleBold}
    onToggleItalic={toggleItalic}
    onToggleStrike={toggleStrike}
    onToggleBulletList={toggleBulletList}
    onToggleOrderedList={toggleOrderedList}
    onToggleTaskList={toggleTaskList}
    onToggleBlockquote={toggleBlockquote}
    onToggleCodeBlock={toggleCodeBlock}
    onInsertTable={insertTable}
    onInsertImage={openImagePicker}
    onInsertHorizontalRule={insertHorizontalRule}
  />

  <div
    class="relative min-h-0 flex-1 overflow-auto bg-background text-foreground"
    bind:this={host}
    role="textbox"
    tabindex="-1"
    aria-label="Éditeur de note"
    data-editor-ready={editorReady}
    data-editor-editable={editorEditable}
    ondragenter={onHostDragEnter}
    ondragleave={onHostDragLeave}
    ondragover={onHostDragOver}
  >
    <input
      type="file"
      accept="image/*"
      class="hidden"
      bind:this={fileInput}
      onchange={onFilePickerChange}
    />
    {#if dragOverCount > 0}
      <div class="pointer-events-none absolute inset-2 z-10 flex items-center justify-center rounded-lg border-2 border-dashed border-accent bg-accent/10 text-sm font-medium text-accent">
        Déposez l'image pour l'insérer
      </div>
    {/if}
    {#if isUploading}
      <div class="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border border-border bg-panel px-3 py-1.5 text-xs text-foreground shadow-md">
        <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-accent"></span>
        Upload en cours…
      </div>
    {/if}
    {#if uploadError}
      <div class="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border border-danger bg-panel px-3 py-1.5 text-xs text-danger shadow-md">
        {uploadError}
      </div>
    {/if}
  </div>
</div>

<!-- Menu flottant ancré au curseur, visible uniquement quand le curseur
     est dans une cellule de tableau. `position: fixed` + coords écran
     évite d'être coupé par l'overflow du conteneur. -->
{#if tableMenuVisible}
  <div
    class="note-table-menu"
    data-placement={tableMenuPlacement}
    style:left="{tableMenuX}px"
    style:top="{tableMenuY}px"
    role="toolbar"
    aria-label="Actions du tableau"
  >
    <button
      type="button"
      title="Ligne au-dessus"
      aria-label="Ligne au-dessus"
      onmousedown={(e) => e.preventDefault()}
      onclick={addRowBefore}
    >
      <ChevronUp size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <button
      type="button"
      title="Ligne en-dessous"
      aria-label="Ligne en-dessous"
      onmousedown={(e) => e.preventDefault()}
      onclick={addRowAfter}
    >
      <ChevronDown size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <span class="note-table-menu__sep" aria-hidden="true"></span>
    <button
      type="button"
      title="Colonne à gauche"
      aria-label="Colonne à gauche"
      onmousedown={(e) => e.preventDefault()}
      onclick={addColumnBefore}
    >
      <ChevronLeft size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <button
      type="button"
      title="Colonne à droite"
      aria-label="Colonne à droite"
      onmousedown={(e) => e.preventDefault()}
      onclick={addColumnAfter}
    >
      <ChevronRight size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <span class="note-table-menu__sep" aria-hidden="true"></span>
    <button
      type="button"
      title="Supprimer la ligne"
      aria-label="Supprimer la ligne"
      onmousedown={(e) => e.preventDefault()}
      onclick={deleteRow}
    >
      <Rows3 size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <button
      type="button"
      title="Supprimer la colonne"
      aria-label="Supprimer la colonne"
      onmousedown={(e) => e.preventDefault()}
      onclick={deleteColumn}
    >
      <Columns3 size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <span class="note-table-menu__sep" aria-hidden="true"></span>
    <button
      type="button"
      title="Supprimer le tableau"
      aria-label="Supprimer le tableau"
      onmousedown={(e) => e.preventDefault()}
      onclick={deleteTable}
    >
      <Trash2 size={14} strokeWidth={2} aria-hidden="true" />
    </button>
  </div>
{/if}

<style>
  .note-table-menu {
    position: fixed;
    z-index: 50;
    display: flex;
    align-items: center;
    gap: 0.125rem;
    padding: 0.25rem;
    background: var(--color-panel);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.25);
  }
  .note-table-menu[data-placement='top']::after,
  .note-table-menu[data-placement='bottom']::after {
    content: '';
    position: absolute;
    left: 8px;
    width: 8px;
    height: 8px;
    background: var(--color-panel);
    border-right: 1px solid var(--color-border);
    border-bottom: 1px solid var(--color-border);
    transform: rotate(45deg);
  }
  .note-table-menu[data-placement='top']::after {
    bottom: -5px;
    border-top: none;
    border-left: none;
  }
  .note-table-menu[data-placement='bottom']::after {
    top: -5px;
    border-bottom: none;
    border-left: none;
  }
  .note-table-menu button {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 1.75rem;
    height: 1.75rem;
    border-radius: var(--radius-sm);
    color: var(--color-subtle);
    background: transparent;
    border: none;
    cursor: pointer;
    transition: background-color 120ms, color 120ms;
  }
  .note-table-menu button:hover {
    background: var(--color-panel-muted);
    color: var(--color-foreground);
  }
  .note-table-menu__sep {
    width: 1px;
    align-self: stretch;
    background: var(--color-border);
    margin: 0.25rem 0.125rem;
  }
  :global(.note-editor-content) {
    min-height: 100%;
    width: 100%;
    padding: 1.25rem 1rem 4rem;
    outline: none;
    background: var(--color-background);
    color: var(--color-foreground);
    line-height: 1.7;
    caret-color: var(--color-foreground);
  }

  :global(.note-editor-content:focus) {
    outline: none;
  }

  :global(.note-editor-content > :first-child) {
    margin-top: 0;
  }

  :global(.note-editor-content h1),
  :global(.note-editor-content h2),
  :global(.note-editor-content h3) {
    margin-top: 1.6rem;
    margin-bottom: 0.7rem;
    color: var(--color-foreground);
    line-height: 1.15;
  }

  :global(.note-editor-content h1) {
    font-size: 2.15rem;
    font-weight: 720;
    letter-spacing: 0;
  }

  :global(.note-editor-content h2) {
    font-size: 1.55rem;
    font-weight: 680;
    letter-spacing: 0;
  }

  :global(.note-editor-content h3) {
    font-size: 1.2rem;
    font-weight: 650;
    letter-spacing: 0;
  }

  :global(.note-editor-content p) {
    margin: 0.75rem 0;
  }

  :global(.note-editor-content strong) {
    font-weight: 700;
  }

  :global(.note-editor-content hr) {
    margin: 1.75rem 0;
    border: 0;
    border-top: 1px solid var(--color-border);
  }

  :global(.note-editor-content ul),
  :global(.note-editor-content ol) {
    margin: 0.75rem 0;
    padding-left: 1.55rem;
  }

  /* Preflight Tailwind v4 met `list-style: none` sur tous les ul/ol :
     on doit restaurer les marqueurs explicitement. Le opt-out taskList
     ci-dessous garde la priorité grâce à son sélecteur d'attribut. */
  :global(.note-editor-content ul) {
    list-style-type: disc;
  }

  :global(.note-editor-content ul ul) {
    list-style-type: circle;
  }

  :global(.note-editor-content ol) {
    list-style-type: decimal;
  }

  :global(.note-editor-content li) {
    margin: 0.25rem 0;
  }

  :global(.note-editor-content li > p) {
    margin: 0.25rem 0;
  }

  :global(.note-editor-content li::marker) {
    color: var(--color-subtle);
  }

  :global(.note-editor-content ul[data-type='taskList']) {
    padding-left: 0;
    list-style: none;
  }

  :global(.note-editor-content ul[data-type='taskList'] ul[data-type='taskList']) {
    padding-left: 1.55rem;
    margin: 0.25rem 0;
  }

  /* Cible `ul[data-type='taskList'] > li` et non `li[data-type='taskItem']` :
     le NodeView de TaskItem ne pose que `data-checked` sur le <li> rendu
     en live (`data-type` n'existe que dans le HTML sérialisé). Le <ul>
     parent, sans NodeView, conserve son attribut. */
  :global(.note-editor-content ul[data-type='taskList'] > li) {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
  }

  :global(.note-editor-content ul[data-type='taskList'] > li > label) {
    flex: 0 0 auto;
    margin-top: 0.18rem;
  }

  :global(.note-editor-content ul[data-type='taskList'] > li > div) {
    flex: 1 1 auto;
    min-width: 0;
  }

  :global(.note-editor-content ul[data-type='taskList'] > li > div > p) {
    margin: 0;
  }

  :global(.note-editor-content input[type='checkbox']) {
    width: 1rem;
    height: 1rem;
    accent-color: var(--color-accent);
  }

  :global(.note-editor-content table) {
    width: 100%;
    margin: 1rem 0;
    border-collapse: collapse;
    overflow: hidden;
    border-radius: var(--radius-md);
  }

  :global(.note-editor-content th),
  :global(.note-editor-content td) {
    min-width: 7rem;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--color-border);
    color: var(--color-foreground);
    vertical-align: top;
  }

  :global(.note-editor-content th) {
    background: var(--color-panel-muted);
    font-weight: 650;
  }

  :global(.note-editor-content pre) {
    overflow-x: auto;
    margin: 1rem 0;
    padding: 0.95rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    background: var(--color-code);
    color: var(--color-foreground);
  }

  :global(.note-editor-content code) {
    padding: 0.08rem 0.24rem;
    border-radius: var(--radius-sm);
    background: var(--color-code);
    color: var(--color-foreground);
    font-size: 0.92em;
  }

  :global(.note-editor-content pre code) {
    padding: 0;
    background: transparent;
    font-size: 0.9rem;
  }

  :global(.note-editor-content blockquote) {
    margin: 1rem 0;
    padding: 0.1rem 0 0.1rem 1rem;
    border-left: 3px solid var(--color-accent);
    color: var(--color-subtle);
  }

  :global(.note-editor-content a) {
    color: var(--color-accent);
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  :global(.note-editor-content .wiki-link) {
    text-decoration: underline;
    text-underline-offset: 0.18em;
    text-decoration-thickness: 1px;
    border-radius: 0.15rem;
    padding: 0 0.05em;
    cursor: pointer;
  }

  :global(.note-editor-content .wiki-link--exists) {
    color: var(--color-accent);
    text-decoration-color: var(--color-accent);
  }

  :global(.note-editor-content .wiki-link--missing) {
    color: var(--color-danger);
    text-decoration-color: var(--color-danger);
    text-decoration-style: dashed;
  }

  :global(.note-editor-content .ProseMirror-selectednode) {
    outline: 2px solid var(--color-focus);
  }

  @media (max-width: 640px) {
    :global(.note-editor-content) {
      padding: 1rem 0.75rem 3rem;
    }
  }
</style>
