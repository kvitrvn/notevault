<script lang="ts">
  import Bold from '@lucide/svelte/icons/bold';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ChevronUp from '@lucide/svelte/icons/chevron-up';
  import Code from '@lucide/svelte/icons/code';
  import Columns3 from '@lucide/svelte/icons/columns-3';
  import Heading1 from '@lucide/svelte/icons/heading-1';
  import Heading2 from '@lucide/svelte/icons/heading-2';
  import Heading3 from '@lucide/svelte/icons/heading-3';
  import ImagePlus from '@lucide/svelte/icons/image-plus';
  import Italic from '@lucide/svelte/icons/italic';
  import List from '@lucide/svelte/icons/list';
  import ListChecks from '@lucide/svelte/icons/list-checks';
  import ListOrdered from '@lucide/svelte/icons/list-ordered';
  import Minus from '@lucide/svelte/icons/minus';
  import Pilcrow from '@lucide/svelte/icons/pilcrow';
  import Quote from '@lucide/svelte/icons/quote';
  import Redo2 from '@lucide/svelte/icons/redo-2';
  import Rows3 from '@lucide/svelte/icons/rows-3';
  import Strikethrough from '@lucide/svelte/icons/strikethrough';
  import TableIcon from '@lucide/svelte/icons/table';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Undo2 from '@lucide/svelte/icons/undo-2';
  import { onDestroy, onMount } from 'svelte';
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
  import {
    isRemoteImageSource,
    isSafeEditorImageSource,
    scrubAbsoluteAssetURLs,
    withTimeout
  } from '../lib/assets';

  export let markdown = '';
  export let onChange: (value: string) => void = () => {};
  export let onDirty: () => void = () => {};
  export let knownTitles: Set<string> = new Set();
  export let onWikiNavigate: (target: string) => void = () => {};
  export let onWikiCreate: (target: string) => void = () => {};
  export let onAssetUpload: (file: File) => Promise<string | null> = async () => null;
  export let onAssetImportFromPath: (absolutePath: string) => Promise<string | null> = async () => null;
  export let assetURL: (relPath: string) => Promise<string> = async (rel) => rel;
  export let onReady: (state: { isEditable: boolean; isFocused: boolean }) => void = () => {};
  export let onError: (error: unknown) => void = () => {};

  type BlockValue = 'paragraph' | 'heading-1' | 'heading-2' | 'heading-3';

  let host: HTMLDivElement;
  let editor: Editor | null = null;
  let lastMarkdown = markdown;
  // Suit la dernière valeur de la prop `markdown` que NoteEditor a vue.
  // Sert à distinguer un vrai changement externe (autre note ouverte,
  // restore historique, recovery) d'une simple réassignation Svelte avec
  // contenu byte-égal (ex. `selected = saved` dans `flushSave`). Sans ce
  // tracker, le bloc réactif `markdown !== lastMarkdown` se déclenchait
  // sporadiquement pendant le cycle save et dispatchait un `replaceWith`
  // complet qui démontait/remontait les décorations wiki-link → flicker.
  let lastSeenPropMarkdown = markdown;
  let editorReady = false;
  let editorEditable = false;
  // Compteur incrémenté à chaque transaction ProseMirror. C'est lui qui
  // sert de signal à Svelte 5 pour re-évaluer les expressions du template
  // dépendant de l'état de l'éditeur (`canUndo`, `canRedo`, `isActive`,
  // etc.). Sans ça, `editor = editor` était un no-op : Svelte compare
  // l'identité des objets en `===` strict, donc une réassignation vers
  // la même référence n'invalide rien et les boutons restent figés.
  let editorTick = 0;
  let pendingChangeTimer: ReturnType<typeof setTimeout> | null = null;
  let hasPendingChange = false;

  // Menu flottant ancré au curseur, visible uniquement quand le curseur
  // est dans une cellule de tableau. Positionné en `fixed` à partir des
  // coords écran fournies par `editor.view.coordsAtPos`.
  let tableMenuVisible = false;
  let tableMenuX = 0;
  let tableMenuY = 0;
  let tableMenuPlacement: 'top' | 'bottom' = 'top';
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

  const iconButtonBase =
    'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border text-subtle transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40';
  const selectBase =
    'h-8 rounded-md border border-transparent bg-transparent px-2 text-sm text-foreground hover:bg-panel-muted focus:bg-panel-muted';

// Toute transaction ProseMirror (saisie, sélection, meta) passe par
// `onTransaction`, donc on centralise la notification ici. On incrémente
// `editorTick` plutôt que `editor = editor` parce que Svelte 5 utilise
// `===` strict pour décider d'invalider une variable legacy : deux
// références identiques vers le même objet ne déclenchent rien, donc les
// expressions du template (`canUndo`, `isActive`, etc.) ne se réévalueraient
// jamais. Le tick est une valeur primitive qui change forcément.
function notifyEditorChange(): void {
  editorTick++;
}

  function iconButtonClass(active = false): string {
    return active
      ? `${iconButtonBase} border-accent bg-accent text-accent-foreground hover:bg-accent-hover hover:text-accent-foreground`
      : `${iconButtonBase} border-transparent bg-transparent hover:bg-panel-muted`;
  }

  function canUndo(): boolean {
    // Lecture de `editorTick` pour que Svelte enregistre la dépendance
    // et re-évalue cette fonction à chaque transaction.
    editorTick;
    return editor?.can().undo() ?? false;
  }

  function canRedo(): boolean {
    editorTick;
    return editor?.can().redo() ?? false;
  }

  function isActive(name: string, attributes: Record<string, unknown> | undefined = undefined): boolean {
    editorTick;
    return editor?.isActive(name, attributes) ?? false;
  }

  function currentBlock(): BlockValue {
    editorTick;
    if (editor?.isActive('heading', { level: 1 })) return 'heading-1';
    if (editor?.isActive('heading', { level: 2 })) return 'heading-2';
    if (editor?.isActive('heading', { level: 3 })) return 'heading-3';

    return 'paragraph';
  }

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
          handlePaste: (view, event) => {
            const items = Array.from(event.clipboardData?.items ?? []);
            const imageItem = items.find((it) => it.type.startsWith('image/'));
            if (!imageItem) return false;
            const file = imageItem.getAsFile();
            if (!file) return false;
            event.preventDefault();
            void handleAssetInsert(file);
            return true;
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

  let isUploading = false;
  let uploadError = '';

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
  $: if (editor && markdown !== lastSeenPropMarkdown) {
    lastSeenPropMarkdown = markdown;
    if (markdown !== lastMarkdown) {
      const ed = editor;
      if (pendingChangeTimer) {
        clearTimeout(pendingChangeTimer);
        pendingChangeTimer = null;
      }
      hasPendingChange = false;
      try {
        const json = ed.markdown?.parse(markdown) ?? { type: 'doc', content: [] };
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
      lastMarkdown = markdown;
    }
  }

  // Rafraîchit les décorations wiki-link quand la liste des titres connus
  // change (création, suppression, ouverture d'une autre note).
  $: if (editor && knownTitles) {
    refreshWikiLinkDecorations(editor);
  }

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

  function changeBlock(event: Event): void {
    const value = (event.currentTarget as HTMLSelectElement).value as BlockValue;
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
  let fileInput: HTMLInputElement | null = null;
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
  let dragOverCount = 0;
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
  <div class="flex shrink-0 items-center gap-0.5 overflow-x-auto bg-background px-3 py-2" aria-label="Outils de mise en forme">
    <button class={iconButtonClass()} type="button" title="Annuler" aria-label="Annuler" onclick={undo} disabled={!canUndo()}>
      <Undo2 size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass()} mr-3`} type="button" title="Rétablir" aria-label="Rétablir" onclick={redo} disabled={!canRedo()}>
      <Redo2 size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <label class="sr-only" for="note-editor-block-type">Style de bloc</label>
    <select id="note-editor-block-type" class={selectBase} value={currentBlock()} onchange={changeBlock} title="Style de bloc">
      <option value="paragraph">Texte</option>
      <option value="heading-1">Titre 1</option>
      <option value="heading-2">Titre 2</option>
      <option value="heading-3">Titre 3</option>
    </select>
    <span class="mr-3 hidden items-center px-1 text-faint sm:inline-flex" aria-hidden="true">
      {#if currentBlock() === 'heading-1'}
        <Heading1 size={16} strokeWidth={2} />
      {:else if currentBlock() === 'heading-2'}
        <Heading2 size={16} strokeWidth={2} />
      {:else if currentBlock() === 'heading-3'}
        <Heading3 size={16} strokeWidth={2} />
      {:else}
        <Pilcrow size={16} strokeWidth={2} />
      {/if}
    </span>

    <button class={iconButtonClass(isActive('bold'))} type="button" title="Gras" aria-label="Gras" aria-pressed={isActive('bold')} onclick={toggleBold}>
      <Bold size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('italic'))} type="button" title="Italique" aria-label="Italique" aria-pressed={isActive('italic')} onclick={toggleItalic}>
      <Italic size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass(isActive('strike'))} mr-3`} type="button" title="Barré" aria-label="Barré" aria-pressed={isActive('strike')} onclick={toggleStrike}>
      <Strikethrough size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <button class={iconButtonClass(isActive('bulletList'))} type="button" title="Liste à puces" aria-label="Liste à puces" aria-pressed={isActive('bulletList')} onclick={toggleBulletList}>
      <List size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('orderedList'))} type="button" title="Liste numérotée" aria-label="Liste numérotée" aria-pressed={isActive('orderedList')} onclick={toggleOrderedList}>
      <ListOrdered size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass(isActive('taskList'))} mr-3`} type="button" title="Liste de tâches" aria-label="Liste de tâches" aria-pressed={isActive('taskList')} onclick={toggleTaskList}>
      <ListChecks size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <button class={iconButtonClass(isActive('blockquote'))} type="button" title="Citation" aria-label="Citation" aria-pressed={isActive('blockquote')} onclick={toggleBlockquote}>
      <Quote size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('codeBlock'))} type="button" title="Bloc de code" aria-label="Bloc de code" aria-pressed={isActive('codeBlock')} onclick={toggleCodeBlock}>
      <Code size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass()} type="button" title="Insérer un tableau" aria-label="Insérer un tableau" onclick={insertTable}>
      <TableIcon size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass()} mr-3`} type="button" title="Insérer une image" aria-label="Insérer une image" onclick={openImagePicker}>
      <ImagePlus size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass()} type="button" title="Insérer une séparation" aria-label="Insérer une séparation" onclick={insertHorizontalRule}>
      <Minus size={16} strokeWidth={2} aria-hidden="true" />
    </button>
  </div>

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

  :global(.note-editor-content li) {
    margin: 0.25rem 0;
  }

  :global(.note-editor-content li::marker) {
    color: var(--color-subtle);
  }

  :global(.note-editor-content ul[data-type='taskList']) {
    padding-left: 0;
    list-style: none;
  }

  :global(.note-editor-content li[data-type='taskItem']) {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
  }

  :global(.note-editor-content li[data-type='taskItem'] > label) {
    flex: 0 0 auto;
    margin-top: 0.18rem;
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
