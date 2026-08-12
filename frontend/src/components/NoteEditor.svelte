<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { createNoteEditor, type NoteEditorHandle } from '../lib/editor/crepe';
  import { isRemoteImageSource, withTimeout } from '../lib/assets';

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
  // Surtout pas `$state` — le proxy profond casserait ProseMirror.
  let editor: NoteEditorHandle | null = $state.raw(null);
  let lastMarkdown = markdown;
  // Suit la dernière valeur de la prop `markdown` que NoteEditor a vue.
  // Sert à distinguer un vrai changement externe (autre note ouverte,
  // restore historique, recovery) d'une simple réassignation Svelte avec
  // contenu byte-égal (ex. `selected = saved` dans `flushSave`).
  let lastSeenPropMarkdown = markdown;
  let editorReady = $state(false);
  let pendingChangeTimer: ReturnType<typeof setTimeout> | null = null;
  let hasPendingChange = false;
  // Vrai pendant un remplacement de contenu piloté par l'hôte : les
  // transactions qui en découlent ne doivent pas être vues comme une saisie.
  let replacingContent = false;

  const MARKDOWN_CHANGE_DEBOUNCE_MS = 200;
  const MAX_ASSET_BYTES = 10 * 1024 * 1024;
  const UPLOAD_TIMEOUT_MS = 15_000;
  const ASSET_URL_TIMEOUT_MS = 5_000;
  const isDev = Boolean((import.meta as ImportMeta & { env?: { DEV?: boolean } }).env?.DEV);

  let isUploading = $state(false);
  let uploadError = $state('');

  function reportUploadError(message: string): void {
    uploadError = message;
    setTimeout(() => (uploadError = ''), 4000);
  }

  function serializeMarkdown(ed: NoteEditorHandle): string {
    if (isDev) console.time('NoteEditor:getMarkdown');
    try {
      return ed.getMarkdown();
    } finally {
      if (isDev) console.timeEnd('NoteEditor:getMarkdown');
    }
  }

  function onDocChanged(): void {
    if (replacingContent) return;
    onDirty();
    hasPendingChange = true;
    if (pendingChangeTimer) clearTimeout(pendingChangeTimer);
    pendingChangeTimer = setTimeout(emitPendingChange, MARKDOWN_CHANGE_DEBOUNCE_MS);
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

  // Les fichiers locaux sont importés par Go. Les URL distantes sont refusées
  // pour préserver la promesse local-first et éviter les pixels de suivi.
  async function importDroppedURI(uri: string): Promise<void> {
    if (!editor) return;

    // Cas particulier : file:// — on délègue au backend Go qui n'a pas la
    // restriction CORS. C'est le cas typique des drops depuis
    // Nautilus/Dolphin sur WebKit Linux.
    if (!/^file:\/\//i.test(uri)) {
      reportUploadError(
        isRemoteImageSource(uri)
          ? 'Images distantes bloquées — téléchargez le fichier avant de l’ajouter'
          : 'Source d’image non locale bloquée'
      );
      return;
    }

    isUploading = true;
    uploadError = '';
    try {
      const absolutePath = decodeURIComponent(uri.replace(/^file:\/\//i, ''));
      const relativePath = await withTimeout(
        onAssetImportFromPath(absolutePath),
        UPLOAD_TIMEOUT_MS,
        'Import timeout (15s)'
      );
      if (!relativePath) return;
      const alt = absolutePath.split('/').pop()?.replace(/\?.*$/, '') || 'image';
      editor.insertAsset(relativePath, alt);
    } catch (err) {
      reportUploadError(`Image distante : ${err}`);
    } finally {
      isUploading = false;
    }
  }

  // Câblé sur `onUpload` de Crepe : coller ou déposer un fichier image, ou
  // utiliser le bouton du bloc image, passe par ici. Retourne le chemin
  // relatif au coffre — jamais une URL absolue, qui finirait dans le .md.
  async function uploadAsset(file: File): Promise<string | null> {
    if (file.size > MAX_ASSET_BYTES) {
      reportUploadError('Image trop volumineuse (>10 MB)');
      return null;
    }
    isUploading = true;
    uploadError = '';
    try {
      return await withTimeout(onAssetUpload(file), UPLOAD_TIMEOUT_MS, 'Upload timeout (15s)');
    } catch (err) {
      reportUploadError(String(err));
      return null;
    } finally {
      isUploading = false;
    }
  }

  onMount(() => {
    if (!host) return;
    let disposed = false;

    void createNoteEditor({
      root: host,
      markdown,
      knownTitles: () => knownTitles,
      onWikiNavigate: (target) => onWikiNavigate(target),
      onWikiCreate: (target) => onWikiCreate(target),
      onAssetUpload: uploadAsset,
      assetURL: (rel) => withTimeout(assetURL(rel), ASSET_URL_TIMEOUT_MS, 'assetURL timeout (5s)'),
      onDocChanged,
      onDropURI: (uri) => {
        void importDroppedURI(uri);
        return true;
      },
      placeholderText: 'Écrire… (« / » pour les commandes)'
    })
      .then((handle) => {
        if (disposed) {
          void handle.destroy();
          return;
        }
        editor = handle;
        editorReady = true;
        onReady({ isEditable: true, isFocused: false });
      })
      .catch((err) => {
        onError(err);
        console.error('[editor] init failed:', err);
      });

    return () => {
      disposed = true;
    };
  });

  // Recharge le contenu de l'éditeur quand la prop `markdown` arrive de
  // l'extérieur (restauration d'historique, recovery buffer). L'ouverture
  // d'une autre note remonte tout le composant via `{#key}` côté App.
  //
  // On compare `markdown` à `lastSeenPropMarkdown` (pas à `lastMarkdown`)
  // pour distinguer :
  //   - vrai changement externe → on recharge le doc ;
  //   - réassignation Svelte byte-égale (`selected = saved` après save) →
  //     on ne fait rien, sinon `replaceAll` démonte les décorations
  //     wiki-link et provoque un flicker visuel.
  //
  // Dépendances trackées : uniquement `markdown` et `editor` (capturés AVANT
  // untrack). Le corps est sous `untrack` parce que le remplacement dispatche
  // des transactions qui repassent par `onDocChanged`.
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
      replacingContent = true;
      try {
        ed.replaceMarkdown(md);
      } catch (err) {
        console.error('[editor] replaceMarkdown failed:', err);
      } finally {
        replacingContent = false;
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
    untrack(() => ed.refreshWikiLinks());
  });

  onDestroy(() => {
    if (pendingChangeTimer) {
      clearTimeout(pendingChangeTimer);
      pendingChangeTimer = null;
    }
    void editor?.destroy();
    editor = null;
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
  function onHostDrop(): void {
    dragOverCount = 0;
  }
</script>

<div class="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background text-foreground">
  <div
    class="relative min-h-0 flex-1 overflow-auto bg-background text-foreground"
    bind:this={host}
    role="textbox"
    tabindex="-1"
    aria-label="Éditeur de note"
    data-editor-ready={editorReady}
    ondragenter={onHostDragEnter}
    ondragleave={onHostDragLeave}
    ondragover={onHostDragOver}
    ondrop={onHostDrop}
  >
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

<style>
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
     on doit restaurer les marqueurs explicitement. Les listes rendues par
     la feature `list-item` de Crepe utilisent leurs propres puces et sont
     ré-neutralisées plus bas. */
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

  /* La feature `list-item` de Crepe rend ses propres marqueurs dans un
     NodeView : on retire ceux du navigateur pour éviter le doublon. */
  :global(.note-editor-content li.milkdown-list-item-block) {
    list-style: none;
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
