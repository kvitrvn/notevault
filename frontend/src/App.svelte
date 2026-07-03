<script lang="ts">
  import Moon from '@lucide/svelte/icons/moon';
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import Sun from '@lucide/svelte/icons/sun';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import NoteEditor from './components/NoteEditor.svelte';
  import { domain } from '../wailsjs/go/models';

  // Généré automatiquement par Wails lors du premier `wails dev`.
  import {
    CreateNote,
    DeleteNote,
    ListNotes,
    OpenNote,
    SaveNote,
    VaultPath
  } from '../wailsjs/go/main/App';

  let notes: domain.NoteSummary[] = [];
  let selected: domain.Note | null = null;
  let vaultPath = '';
  let loading = true;
  let saving = false;
  let deleting = false;
  let confirmingDelete = false;
  let error = '';
  let theme: 'dark' | 'light' =
    document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';

  $: currentTitle = selected?.title?.trim() || 'Aucune note';
  $: selectedPath = selected?.relativePath || '';

  function formatDate(value: unknown): string {
    const date = new Date(String(value ?? ''));

    return Number.isNaN(date.getTime())
      ? ''
      : date.toLocaleDateString('fr-FR', {
          day: '2-digit',
          month: '2-digit',
          year: 'numeric'
        });
  }

  async function refresh(): Promise<void> {
    loading = true;
    error = '';

    try {
      [notes, vaultPath] = await Promise.all([ListNotes(), VaultPath()]);
    } catch (err) {
      error = String(err);
    } finally {
      loading = false;
    }
  }

  async function openNote(relativePath: string): Promise<void> {
    error = '';

    try {
      selected = await OpenNote(relativePath);
    } catch (err) {
      error = String(err);
    }
  }

  async function createNote(templateKey = ''): Promise<void> {
    error = '';

    try {
      selected = await CreateNote('Nouvelle note', templateKey);
      await refresh();
    } catch (err) {
      error = String(err);
    }
  }

  function updateContent(content: string): void {
    if (!selected) return;

    selected.content = content;
    selected = selected;
  }

  async function saveSelected(): Promise<void> {
    if (!selected || saving) return;

    saving = true;
    error = '';

    try {
      selected = await SaveNote(selected);
      await refresh();
    } catch (err) {
      error = String(err);
    } finally {
      saving = false;
    }
  }

  function requestDelete(): void {
    if (!selected || deleting) return;

    confirmingDelete = true;
  }

  function cancelDelete(): void {
    if (deleting) return;

    confirmingDelete = false;
  }

  async function confirmDelete(): Promise<void> {
    if (!selected || deleting) return;

    const relativePath = selected.relativePath;
    deleting = true;
    error = '';

    try {
      await DeleteNote(relativePath);
      if (selected?.relativePath === relativePath) {
        selected = null;
      }
      confirmingDelete = false;
      await refresh();
    } catch (err) {
      error = String(err);
    } finally {
      deleting = false;
    }
  }

  function toggleTheme(): void {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem('notevault-theme', theme);
  }

  function onGlobalKeydown(event: KeyboardEvent): void {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      event.preventDefault();
      void saveSelected();
    }
  }

  void refresh();
</script>

<svelte:window onkeydown={onGlobalKeydown} />

<div class="grid h-dvh min-h-0 grid-rows-[14rem_minmax(0,1fr)] bg-background text-foreground lg:grid-cols-[18rem_minmax(0,1fr)] lg:grid-rows-none">
  <aside class="flex min-h-0 flex-col border-b border-border bg-sidebar lg:border-b-0 lg:border-r">
    <div class="flex h-14 shrink-0 items-center justify-between border-b border-border px-4">
      <div class="min-w-0">
        <h1 class="truncate text-sm font-semibold tracking-normal">NoteVault</h1>
        <p class="truncate text-xs text-subtle" title={vaultPath}>
          {vaultPath || 'Chargement du coffre...'}
        </p>
      </div>
      <span class="ml-3 rounded-md border border-border-strong px-2 py-0.5 text-xs text-subtle">
        {notes.length}
      </span>
    </div>

    <div class="flex min-h-0 flex-1 flex-col px-3 py-3">
      <div class="mb-2 flex items-center justify-between gap-2 px-1">
        <h2 class="text-xs font-semibold uppercase text-subtle">Notes</h2>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          title="Nouvelle note"
          aria-label="Nouvelle note"
          onclick={() => createNote()}
        >
          <Plus size={16} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>

      {#if loading}
        <div class="rounded-lg border border-border bg-panel-muted px-3 py-3 text-sm text-subtle">
          Chargement...
        </div>
      {:else if notes.length === 0}
        <div class="rounded-lg border border-dashed border-border-strong bg-panel-muted px-3 py-3 text-sm leading-6 text-subtle">
          Aucune note. Créez-en une pour démarrer.
        </div>
      {:else}
        <nav class="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto pr-1" aria-label="Liste des notes">
          {#each notes as note (note.relativePath)}
            <button
              type="button"
              class={selected?.relativePath === note.relativePath
                ? 'grid w-full gap-1 rounded-lg border border-accent bg-panel px-3 py-2 text-left shadow-sm'
                : 'grid w-full gap-1 rounded-lg border border-transparent px-3 py-2 text-left hover:border-border hover:bg-panel-muted'}
              aria-current={selected?.relativePath === note.relativePath ? 'page' : undefined}
              onclick={() => openNote(note.relativePath)}
            >
              <span class="truncate text-sm font-medium text-foreground">{note.title || 'Sans titre'}</span>
              <span class="truncate text-xs text-subtle">{formatDate(note.updatedAt)}</span>
            </button>
          {/each}
        </nav>
      {/if}
    </div>
  </aside>

  <main class="grid min-h-0 grid-rows-[3.5rem_minmax(0,1fr)] bg-background">
    <header class="flex min-w-0 items-center justify-between gap-3 border-b border-border bg-panel px-4">
      <div class="min-w-0">
        <nav class="flex min-w-0 items-center gap-2 text-xs text-subtle" aria-label="Fil d'Ariane">
          <span class="shrink-0">NoteVault</span>
          <span aria-hidden="true">/</span>
          <span class="truncate text-foreground">{currentTitle}</span>
        </nav>
        <p class="mt-0.5 truncate text-xs text-faint" title={selectedPath || vaultPath}>
          {selectedPath || vaultPath || 'Coffre local'}
        </p>
      </div>

      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
        type="button"
        title={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
        aria-label={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
        aria-pressed={theme === 'dark'}
        onclick={toggleTheme}
      >
        {#if theme === 'dark'}
          <Sun size={17} strokeWidth={2} aria-hidden="true" />
        {:else}
          <Moon size={17} strokeWidth={2} aria-hidden="true" />
        {/if}
      </button>
    </header>

    <section class="flex min-h-0 flex-col overflow-hidden" aria-label="Éditeur de note">
      {#if error}
        <p class="mx-4 mt-4 rounded-lg border border-danger/40 bg-panel px-3 py-2 text-sm text-danger" role="alert">
          {error}
        </p>
      {/if}

      {#if selected}
        <div class="flex min-h-0 flex-1 flex-col">
          <input
            class="block w-full shrink-0 border-0 bg-transparent px-4 pb-3 pt-5 text-3xl font-semibold leading-tight text-foreground outline-none placeholder:text-faint focus:outline-none focus-visible:outline-none sm:text-4xl"
            aria-label="Titre de la note"
            bind:value={selected.title}
            placeholder="Sans titre"
          />

          <div class="min-h-0 flex-1">
            <NoteEditor markdown={selected.content} onChange={updateContent} />
          </div>

          <footer class="flex min-h-12 items-center justify-between gap-3 border-t border-border bg-panel px-4 text-xs text-faint">
            <span class="truncate" title={selected.relativePath}>{selected.relativePath}</span>
            <div class="flex shrink-0 items-center gap-2">
              <button
                class="inline-flex items-center gap-2 rounded-md border border-danger/45 bg-transparent px-3 py-1.5 text-sm font-medium text-danger hover:bg-danger/10 disabled:hover:bg-transparent"
                type="button"
                onclick={requestDelete}
                disabled={deleting || saving}
              >
                <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                Supprimer
              </button>
              <button
                class="inline-flex items-center gap-2 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover disabled:hover:bg-accent"
                type="button"
                onclick={saveSelected}
                disabled={saving || deleting}
              >
                <Save size={15} strokeWidth={2} aria-hidden="true" />
                {saving ? 'Enregistrement...' : 'Enregistrer'}
              </button>
            </div>
          </footer>
        </div>
      {:else}
        <div class="grid min-h-0 flex-1 place-items-center rounded-lg border border-dashed border-border-strong bg-panel-muted p-8 text-center">
          <div class="max-w-md">
            <h2 class="text-2xl font-semibold text-foreground">Vos notes vous appartiennent.</h2>
            <p class="mt-2 text-sm leading-6 text-subtle">
              Les fichiers Markdown sont enregistrés dans votre coffre local.
            </p>
          </div>
        </div>
      {/if}
    </section>
  </main>
</div>

{#if confirmingDelete && selected}
  <div class="fixed inset-0 z-50 grid place-items-center px-4">
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer la confirmation"
      onclick={cancelDelete}
      disabled={deleting}
    ></button>
    <div
      class="relative w-full max-w-sm rounded-lg border border-border bg-panel p-4 shadow-xl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-note-title"
    >
      <h2 id="delete-note-title" class="text-base font-semibold text-foreground">
        Supprimer cette note ?
      </h2>
      <p class="mt-2 text-sm leading-6 text-subtle">
        Cette action supprimera le fichier Markdown du coffre local.
      </p>
      <p class="mt-3 truncate rounded-md bg-background px-2 py-1 text-xs text-faint" title={selected.relativePath}>
        {selected.relativePath}
      </p>
      <div class="mt-4 flex justify-end gap-2">
        <button
          class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          onclick={cancelDelete}
          disabled={deleting}
        >
          Annuler
        </button>
        <button
          class="inline-flex items-center gap-2 rounded-md border border-danger/45 bg-danger px-3 py-1.5 text-sm font-medium text-background hover:opacity-90"
          type="button"
          onclick={confirmDelete}
          disabled={deleting}
        >
          <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
          {deleting ? 'Suppression...' : 'Supprimer'}
        </button>
      </div>
    </div>
  </div>
{/if}
