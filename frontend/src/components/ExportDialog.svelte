<script lang="ts">
  import Download from '@lucide/svelte/icons/download';
  import X from '@lucide/svelte/icons/x';
  import { ExportNotes } from '../../wailsjs/go/main/App';
  import type { domain } from '../../wailsjs/go/models';

  type NoteSummary = domain.NoteSummary;

  type Props = {
    open: boolean;
    notes: NoteSummary[];
    defaultFilename: string;
    onClose: () => void;
    onSuccess: (path: string) => void;
  };

  let { open, notes, defaultFilename, onClose, onSuccess }: Props = $props();

  let selected = $state<Set<string>>(new Set());
  let filename = $state('');
  let busy = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      selected = new Set();
      filename = defaultFilename;
      error = '';
    }
  });

  function toggle(rel: string): void {
    if (selected.has(rel)) selected.delete(rel);
    else selected.add(rel);
    selected = new Set(selected);
  }

  function selectAll(): void {
    selected = new Set(notes.map((n) => n.relativePath));
  }

  function clearAll(): void {
    selected = new Set();
  }

  async function commit(): Promise<void> {
    if (selected.size === 0) {
      error = 'Sélectionnez au moins une note.';
      return;
    }
    const name = filename.trim() || defaultFilename;
    const safe = name.endsWith('.zip') ? name : name + '.zip';
    busy = true;
    error = '';
    try {
      const list = Array.from(selected);
      await ExportNotes(list, safe);
      onSuccess(safe);
      onClose();
    } catch (err) {
      error = String(err);
    } finally {
      busy = false;
    }
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      void commit();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 py-10"
    role="dialog"
    aria-modal="true"
    aria-labelledby="export-title"
  >
    <button
      class="fixed inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative mx-auto flex max-h-[80vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center justify-between gap-2 border-b border-border bg-background px-4 py-3">
        <h2 id="export-title" class="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <Download size={15} strokeWidth={2} class="text-accent" aria-hidden="true" />
          Exporter vers un zip
        </h2>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
          aria-label="Fermer"
          onclick={onClose}
        >
          <X size={13} strokeWidth={2} aria-hidden="true" />
        </button>
      </header>

      <div class="flex min-h-0 flex-1 flex-col gap-3 px-4 py-3">
        <p class="text-xs text-subtle">
          Sélectionnez les notes à inclure. Les images référencées
          (<code class="rounded bg-background px-1 py-0.5">assets/...</code>)
          sont ajoutées automatiquement.
        </p>

        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border bg-background px-2.5 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
            onclick={selectAll}
            disabled={notes.length === 0}
          >
            Tout
          </button>
          <button
            type="button"
            class="rounded-md border border-border bg-background px-2.5 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
            onclick={clearAll}
            disabled={selected.size === 0}
          >
            Aucun
          </button>
          <span class="ml-auto text-xs text-faint">{selected.size} / {notes.length}</span>
        </div>

        <ul
          class="min-h-0 flex-1 overflow-y-auto rounded-md border border-border bg-background"
          role="listbox"
          aria-multiselectable="true"
          aria-label="Notes à exporter"
        >
          {#each notes as note (note.relativePath)}
            {@const checked = selected.has(note.relativePath)}
            <li>
              <label
                class="flex cursor-pointer items-center gap-2 border-b border-border/50 px-3 py-1.5 text-sm last:border-b-0 hover:bg-panel-muted"
              >
                <input
                  type="checkbox"
                  class="h-3.5 w-3.5 accent-accent"
                  checked={checked}
                  onchange={() => toggle(note.relativePath)}
                />
                <span class="min-w-0 flex-1 truncate">
                  <span class="text-foreground">{note.title || 'Sans titre'}</span>
                  <span class="ml-2 text-[0.7rem] text-faint">{note.relativePath}</span>
                </span>
              </label>
            </li>
          {/each}
          {#if notes.length === 0}
            <li class="px-3 py-4 text-center text-sm text-subtle">Aucune note à exporter.</li>
          {/if}
        </ul>

        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-subtle">Nom du fichier (écrit à la racine du coffre)</span>
          <input
            type="text"
            bind:value={filename}
            class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
            spellcheck="false"
            autocomplete="off"
            placeholder={defaultFilename}
          />
        </label>

        {#if error}
          <p class="rounded-md border border-danger/40 bg-panel px-3 py-2 text-xs text-danger" role="alert">
            {error}
          </p>
        {/if}
      </div>

      <footer class="flex items-center justify-between gap-2 border-t border-border bg-background px-4 py-3 text-xs text-subtle">
        <span><kbd class="rounded border border-border-strong bg-panel px-1.5 py-0.5">Ctrl+Enter</kbd> pour exporter</span>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
            onclick={onClose}
            disabled={busy}
          >
            Annuler
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
            onclick={() => void commit()}
            disabled={busy || selected.size === 0}
          >
            <Download size={13} strokeWidth={2} aria-hidden="true" />
            {busy ? 'Export…' : 'Exporter'}
          </button>
        </div>
      </footer>
    </div>
  </div>
{/if}
