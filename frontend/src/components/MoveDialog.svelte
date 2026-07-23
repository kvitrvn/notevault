<script lang="ts">
  import FolderInput from '@lucide/svelte/icons/folder-input';
  import X from '@lucide/svelte/icons/x';
  import { normalizeNotesFolderPath } from '../lib/note-paths';

  type FolderInfo = { path: string; name: string; count: number };

  type Props = {
    open: boolean;
    currentPath: string;
    folders: FolderInfo[];
    onMove: (newPath: string) => void;
    onClose: () => void;
  };

  let { open, currentPath, folders, onMove, onClose }: Props = $props();

  const initial = $derived(defaultDestination(currentPath));
  let destination = $state('');
  let pathEl: HTMLInputElement | undefined = $state();

  $effect(() => {
    if (open) {
      destination = initial;
      requestAnimationFrame(() => pathEl?.focus());
    }
  });

  function defaultDestination(rel: string): string {
    const base = rel.split('/').slice(0, -1).join('/');
    if (base === 'notes' || base === '') return 'notes/inbox/';
    return base + '/';
  }

  function commit(): void {
    const v = destination.trim();
    if (!v) return;
    destination = normalizeNotesFolderPath(v) + '/';
    onMove(destination);
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    } else if (event.key === 'Enter') {
      event.preventDefault();
      commit();
    }
  }

  function previewFilename(rel: string, dest: string): string {
    const base = rel.split('/').pop() ?? 'note.md';
    return dest + base;
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[12vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Déplacer la note"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative w-full max-w-md overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <h2 class="flex items-center gap-1.5 text-base font-semibold text-foreground">
          <FolderInput size={16} strokeWidth={2} aria-hidden="true" />
          Déplacer la note
        </h2>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          aria-label="Fermer"
          onclick={onClose}
        >
          <X size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>

      <div class="flex flex-col gap-3 px-4 py-3">
        <p class="text-xs text-subtle">
          Saisissez un dossier de destination (chemin relatif au coffre, sans
          le nom de fichier).
        </p>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-subtle">Dossier de destination</span>
          <input
            bind:this={pathEl}
            type="text"
            bind:value={destination}
            placeholder="notes/projects/web/"
            class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
            spellcheck="false"
            autocomplete="off"
          />
          {#if currentPath}
            <span class="mt-1 truncate text-xs text-faint">
              Nouveau chemin : <code class="rounded bg-background px-1.5 py-0.5">{previewFilename(currentPath, destination)}</code>
            </span>
          {/if}
        </label>

        {#if folders.length > 0}
          <div class="flex flex-col gap-1">
            <span class="text-xs font-medium text-subtle">Dossiers existants</span>
            <div class="flex flex-wrap gap-1">
              {#each folders as f (f.path)}
                <button
                  type="button"
                  class="rounded-full border border-border bg-background px-2 py-0.5 text-xs text-foreground hover:border-accent hover:bg-accent/10"
                  onclick={() => (destination = 'notes/' + f.path + '/')}
                >
                  {f.path}
                </button>
              {/each}
            </div>
          </div>
        {/if}
      </div>

      <div class="flex items-center justify-end gap-2 border-t border-border bg-background px-4 py-2.5">
        <button
          class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          onclick={onClose}
        >
          Annuler
        </button>
        <button
          class="inline-flex items-center gap-2 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
          type="button"
          onclick={commit}
        >
          <FolderInput size={13} strokeWidth={2} aria-hidden="true" />
          Déplacer
        </button>
      </div>
    </div>
  </div>
{/if}
