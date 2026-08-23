<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte';
  import { createNoteEditor, type NoteEditorHandle } from '../lib/editor/crepe';
  import { isRemoteImageSource, withTimeout } from '../lib/assets';
  import {
    PERF_ENABLED,
    createFrameMonitor,
    formatFrameStats,
    logProbeReport,
    probe
  } from '../lib/perf-probe';

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

  let isUploading = $state(false);
  let uploadError = $state('');

  function reportUploadError(message: string): void {
    uploadError = message;
    setTimeout(() => (uploadError = ''), 4000);
  }

  function serializeMarkdown(ed: NoteEditorHandle): string {
    return probe('getMarkdown', () => ed.getMarkdown());
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
      reportUploadError(`Échec de l’import : ${err}`);
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

  // Le défilement se fait sur l'hôte, pas sur `.ProseMirror` : c'est donc là
  // que se mesure la saccade réellement perçue.
  const frameMonitor = createFrameMonitor({
    onReport: (stats) => console.log(formatFrameStats('scroll', stats))
  });

  function onHostScroll(): void {
    frameMonitor.ping();
  }

  onMount(() => {
    if (!host) return;
    let disposed = false;
    const createPerf = PERF_ENABLED ? performance.now() : 0;

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
        if (PERF_ENABLED) {
          console.log(
            `[perf] createNoteEditor — ${(performance.now() - createPerf).toFixed(1)} ms`
          );
          // Émis ici, pas dans `openNote` : le parse Markdown et le montage
          // des blocs ont lieu après le retour de `openNote`, via le remontage
          // du composant par `{#key}`. Le `setTimeout` laisse passer les effets
          // Svelte du montage (dont le premier calcul des décorations
          // wiki-link), qui font partie du coût d'ouverture.
          setTimeout(() => logProbeReport('note ouverte'), 0);
        }
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
    frameMonitor.stop();
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
    role="group"
    aria-label="Zone d’édition et d’import d’images"
    data-editor-ready={editorReady}
    ondragenter={onHostDragEnter}
    ondragleave={onHostDragLeave}
    ondragover={onHostDragOver}
    ondrop={onHostDrop}
    onscroll={PERF_ENABLED ? onHostScroll : undefined}
  >
    {#if dragOverCount > 0}
      <div class="pointer-events-none absolute inset-2 z-10 flex items-center justify-center rounded-lg border-2 border-dashed border-accent bg-accent/10 text-sm font-medium text-accent">
        Déposez l'image pour l'insérer
      </div>
    {/if}
    {#if isUploading}
      <div
        class="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border border-border bg-panel px-3 py-1.5 text-xs text-foreground shadow-md"
        role="status"
        aria-live="polite"
      >
        <span class="inline-block h-2 w-2 animate-pulse rounded-full bg-accent"></span>
        Import de l’image en cours…
      </div>
    {/if}
    {#if uploadError}
      <div
        class="pointer-events-none absolute right-3 top-3 z-20 flex items-center gap-2 rounded-md border border-danger bg-panel px-3 py-1.5 text-xs text-danger shadow-md"
        role="alert"
      >
        {uploadError}
      </div>
    {/if}
  </div>
</div>

<style>
  :global(.milkdown .ProseMirror.note-editor-content) {
    --editor-gutter: clamp(0.75rem, 3vw, 2rem);
    --editor-reading-width: 46rem;

    display: grid;
    grid-template-columns:
      minmax(var(--editor-gutter), 1fr)
      minmax(0, var(--editor-reading-width))
      minmax(var(--editor-gutter), 1fr);
    align-content: start;
    min-height: 100%;
    width: 100%;
    padding: 1.25rem 0 4rem;
    outline: none;
    background: var(--color-background);
    color: var(--color-foreground);
    line-height: 1.7;
    caret-color: var(--color-foreground);
  }

  :global(.milkdown .ProseMirror.note-editor-content > *) {
    grid-column: 2;
    min-width: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content > .milkdown-image-block),
  :global(.milkdown .ProseMirror.note-editor-content > .milkdown-table-block) {
    grid-column: 1 / -1;
    justify-self: center;
    width: calc(100% - 2 * var(--editor-gutter));
  }

  :global(.milkdown .ProseMirror.note-editor-content:focus) {
    outline: none;
  }

  :global(.milkdown .ProseMirror.note-editor-content > :first-child) {
    margin-top: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content h1),
  :global(.milkdown .ProseMirror.note-editor-content h2),
  :global(.milkdown .ProseMirror.note-editor-content h3),
  :global(.milkdown .ProseMirror.note-editor-content h4),
  :global(.milkdown .ProseMirror.note-editor-content h5),
  :global(.milkdown .ProseMirror.note-editor-content h6) {
    padding: 0;
    margin-top: 1.5rem;
    margin-bottom: 0.55rem;
    color: var(--color-foreground);
    line-height: 1.25;
  }

  :global(.milkdown .ProseMirror.note-editor-content h1) {
    font-size: 2rem;
    font-weight: 720;
    line-height: 1.15;
  }

  :global(.milkdown .ProseMirror.note-editor-content h2) {
    font-size: 1.6rem;
    font-weight: 680;
  }

  :global(.milkdown .ProseMirror.note-editor-content h3) {
    font-size: 1.3rem;
    font-weight: 650;
  }

  :global(.milkdown .ProseMirror.note-editor-content h4) {
    font-size: 1.125rem;
    font-weight: 650;
  }

  :global(.milkdown .ProseMirror.note-editor-content h5) {
    font-size: 1rem;
    font-weight: 650;
  }

  :global(.milkdown .ProseMirror.note-editor-content h6) {
    font-size: 0.875rem;
    font-weight: 700;
  }

  :global(.milkdown .ProseMirror.note-editor-content p) {
    margin: 0.65rem 0;
    padding: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content strong) {
    font-weight: 700;
  }

  :global(.milkdown .ProseMirror.note-editor-content hr) {
    height: 1px;
    margin: 1.75rem 0;
    padding: 0;
    border: 0;
    background: var(--color-border);
  }

  :global(.milkdown .ProseMirror.note-editor-content ul),
  :global(.milkdown .ProseMirror.note-editor-content ol) {
    margin: 0.65rem 0;
    padding: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block) {
    margin: 0;
    padding: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block > .list-item) {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin: 0.15rem 0;
    padding: 0;
    list-style: none;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block > .list-item > .label-wrapper) {
    display: flex;
    align-items: flex-start;
    justify-content: flex-end;
    flex: 0 0 1.25rem;
    width: 1.25rem;
    height: auto;
    min-height: 1.7em;
    color: var(--color-subtle);
    line-height: inherit;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block > .list-item > .label-wrapper > .label) {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex: none;
    width: 100%;
    height: 1.7em;
    padding: 0;
    line-height: 1;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block > .list-item > .label-wrapper > .label.ordered) {
    width: max-content;
    min-width: 100%;
    justify-content: flex-end;
    font-variant-numeric: tabular-nums;
    white-space: nowrap;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block > .list-item > .label-wrapper > .label svg) {
    display: block;
    width: 1rem;
    height: 1rem;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block .children) {
    min-width: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block .children > .content-dom > p:first-child) {
    margin-top: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block .children > .content-dom > p:last-child) {
    margin-bottom: 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block .children > .content-dom > ul),
  :global(.milkdown .ProseMirror.note-editor-content .milkdown-list-item-block .children > .content-dom > ol) {
    margin: 0.25rem 0 0;
  }

  :global(.milkdown .ProseMirror.note-editor-content input[type='checkbox']) {
    width: 1rem;
    height: 1rem;
    accent-color: var(--color-accent);
  }

  :global(.milkdown .ProseMirror.note-editor-content .milkdown-table-block) {
    max-width: 100%;
    margin-block: 1rem;
    overflow-x: auto;
    overscroll-behavior-inline: contain;
  }

  :global(.milkdown .ProseMirror.note-editor-content table) {
    width: max-content;
    min-width: 100%;
    margin: 0;
    border-collapse: collapse;
    table-layout: auto;
    overflow: visible;
  }

  :global(.milkdown .ProseMirror.note-editor-content th),
  :global(.milkdown .ProseMirror.note-editor-content td) {
    min-width: 7rem;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--color-border);
    color: var(--color-foreground);
    vertical-align: top;
  }

  :global(.milkdown .ProseMirror.note-editor-content th) {
    background: var(--color-panel-muted);
    font-weight: 650;
  }

  :global(.milkdown .ProseMirror.note-editor-content pre) {
    overflow-x: auto;
    margin: 1rem 0;
    padding: 0.75rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-code);
    color: var(--color-foreground);
    font-size: 0.85rem;
    line-height: 1.45;
  }

  /* Padding haut réduit : le `.language-button` de Crepe garde sa place même
     masqué (`opacity: 0`), il fournit déjà l'essentiel de la marge du haut. */
  :global(.milkdown .ProseMirror.note-editor-content .milkdown-code-block) {
    overflow-x: auto;
    margin: 1rem 0;
    padding: 0.5rem 0.85rem 0.7rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-code);
  }

  :global(.milkdown .ProseMirror.note-editor-content code) {
    padding: 0.08rem 0.24rem;
    border-radius: var(--radius-sm);
    background: var(--color-code);
    color: var(--color-foreground);
    font-size: 0.92em;
  }

  :global(.milkdown .ProseMirror.note-editor-content pre code) {
    padding: 0;
    background: transparent;
    font-size: inherit;
  }

  :global(.milkdown .ProseMirror.note-editor-content blockquote) {
    position: relative;
    box-sizing: border-box;
    margin: 1rem 0;
    padding: 0.1rem 0 0.1rem 0.9rem;
    border-left: 3px solid var(--color-accent);
    color: var(--color-subtle);
  }

  :global(.milkdown .ProseMirror.note-editor-content blockquote::before) {
    content: none;
  }

  :global(.milkdown .ProseMirror.note-editor-content a) {
    color: var(--color-accent);
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  :global(.milkdown .ProseMirror.note-editor-content .wiki-link) {
    text-decoration: underline;
    text-underline-offset: 0.18em;
    text-decoration-thickness: 1px;
    border-radius: 0.15rem;
    padding: 0 0.05em;
    cursor: pointer;
  }

  :global(.milkdown .ProseMirror.note-editor-content .wiki-link--exists) {
    color: var(--color-accent);
    text-decoration-color: var(--color-accent);
  }

  :global(.milkdown .ProseMirror.note-editor-content .wiki-link--missing) {
    color: var(--color-danger);
    text-decoration-color: var(--color-danger);
    text-decoration-style: dashed;
  }

  :global(.milkdown .ProseMirror.note-editor-content .ProseMirror-selectednode) {
    outline: 1px solid var(--color-focus);
    outline-offset: 2px;
    background: color-mix(in srgb, var(--color-selection), transparent 55%);
  }

  :global(.milkdown .ProseMirror.note-editor-content .list-item.ProseMirror-selectednode) {
    border-radius: var(--radius-sm);
  }

  @media (max-width: 640px) {
    :global(.milkdown .ProseMirror.note-editor-content) {
      padding-top: 1rem;
      padding-bottom: 3rem;
    }
  }
</style>
