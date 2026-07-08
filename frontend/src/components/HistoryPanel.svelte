<script lang="ts">
  import History from '@lucide/svelte/icons/history';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import X from '@lucide/svelte/icons/x';
  import GitCompare from '@lucide/svelte/icons/git-compare';
  import Loader2 from '@lucide/svelte/icons/loader';

  type HistoryEntry = {
    id: string;
    timestamp: string;
    path: string;
    size: number;
    preview: string;
  };

  type Props = {
    open: boolean;
    relativePath: string;
    onRestore: (versionID: string) => Promise<void>;
    onClose: () => void;
  };

  let { open, relativePath, onRestore, onClose }: Props = $props();

  let versions: HistoryEntry[] = $state([]);
  let loading = $state(false);
  let selectedA = $state<string | null>(null);
  let selectedB = $state<string | null>(null);
  let diffContent = $state('');
  let diffLoading = $state(false);
  let restoring = $state<string | null>(null);
  let loaded = '';

  $effect(() => {
    if (!open || !relativePath) {
      versions = [];
      selectedA = null;
      selectedB = null;
      diffContent = '';
      loaded = '';
      return;
    }
    if (loaded === relativePath) return;
    loaded = relativePath;
    void load();
  });

  async function load(): Promise<void> {
    loading = true;
    try {
      const { ListHistory } = await import('../../wailsjs/go/main/App');
      versions = (await ListHistory(relativePath)) ?? [];
      if (versions.length >= 2) {
        selectedA = versions[1].id;
        selectedB = versions[0].id;
      } else if (versions.length === 1) {
        selectedA = versions[0].id;
        selectedB = null;
      }
    } catch (err) {
      console.error('ListHistory', err);
      versions = [];
    } finally {
      loading = false;
    }
  }

  async function computeDiff(): Promise<void> {
    if (!selectedA || !selectedB) {
      diffContent = '';
      return;
    }
    diffLoading = true;
    try {
      const { DiffHistory } = await import('../../wailsjs/go/main/App');
      diffContent = await DiffHistory(relativePath, selectedA, selectedB);
    } catch (err) {
      diffContent = `Erreur : ${err}`;
    } finally {
      diffLoading = false;
    }
  }

  $effect(() => {
    if (selectedA && selectedB) {
      void computeDiff();
    }
  });

  function formatTimestamp(id: string): string {
    // ID = nanosecond timestamp
    const n = Number(id);
    if (!Number.isFinite(n)) return id;
    return new Date(n / 1_000_000).toLocaleString('fr-FR', {
      day: '2-digit',
      month: '2-digit',
      year: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} o`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} ko`;
    return `${(bytes / 1024 / 1024).toFixed(1)} Mo`;
  }

  async function restore(versionID: string): Promise<void> {
    restoring = versionID;
    try {
      await onRestore(versionID);
    } finally {
      restoring = null;
    }
  }
</script>

<svelte:window
  onkeydown={(e) => {
    if (open && e.key === 'Escape') onClose();
  }}
/>

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[8vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Historique de la note"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative flex h-[80vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <h2 class="flex items-center gap-1.5 text-base font-semibold text-foreground">
          <History size={16} strokeWidth={2} aria-hidden="true" />
          Historique
          <span class="ml-1 text-xs font-normal text-subtle" title={relativePath}>
            ({relativePath.split('/').pop()})
          </span>
        </h2>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          aria-label="Fermer"
          title="Fermer (Esc)"
          onclick={onClose}
        >
          <X size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>

      <div class="grid min-h-0 flex-1 grid-cols-1 md:grid-cols-[18rem_minmax(0,1fr)]">
        <aside class="min-h-0 overflow-y-auto border-b border-border md:border-b-0 md:border-r">
          {#if loading}
            <div class="flex items-center gap-2 px-3 py-3 text-sm text-subtle">
              <Loader2 size={12} strokeWidth={2} class="animate-spin" aria-hidden="true" />
              Chargement…
            </div>
          {:else if versions.length === 0}
            <p class="px-3 py-3 text-sm text-subtle">
              Aucune version archivée. Les versions sont créées à chaque
              sauvegarde de la note.
            </p>
          {:else}
            <ul class="flex flex-col">
              {#each versions as v (v.id)}
                {@const isA = selectedA === v.id}
                {@const isB = selectedB === v.id}
                <li
                  class={isA || isB
                    ? 'border-l-2 border-accent bg-accent/10'
                    : 'border-l-2 border-transparent hover:bg-panel-muted'}
                >
                  <button
                    type="button"
                    class="flex w-full flex-col gap-0.5 px-3 py-2 text-left"
                    onclick={(ev: MouseEvent) => {
                      if (ev.shiftKey) {
                        selectedA = v.id;
                      } else if (ev.altKey) {
                        selectedB = v.id;
                      } else {
                        if (selectedA === null) selectedA = v.id;
                        else if (selectedB === null) selectedB = v.id;
                        else {
                          selectedA = selectedB;
                          selectedB = v.id;
                        }
                      }
                    }}
                  >
                    <span class="flex items-center gap-1.5 text-xs text-foreground">
                      {#if isA}
                        <span
                          class="inline-flex h-4 w-4 items-center justify-center rounded bg-accent text-[10px] font-bold text-accent-foreground"
                          aria-label="version A">A</span
                        >
                      {/if}
                      {#if isB}
                        <span
                          class="inline-flex h-4 w-4 items-center justify-center rounded bg-accent text-[10px] font-bold text-accent-foreground"
                          aria-label="version B">B</span
                        >
                      {/if}
                      <span>{formatTimestamp(v.id)}</span>
                    </span>
                    <span class="truncate text-xs text-subtle">{v.preview || '(corps vide)'}</span>
                    <span class="text-[10px] text-faint">{formatSize(v.size)}</span>
                  </button>
                  <div class="flex items-center justify-end gap-1 px-2 pb-1">
                    <button
                      class="inline-flex items-center gap-1 rounded-md border border-border bg-background px-1.5 py-0.5 text-xs text-foreground hover:bg-panel-muted disabled:opacity-50"
                      type="button"
                      title="Marquer comme version A"
                      onclick={() => (selectedA = v.id)}
                    >A</button>
                    <button
                      class="inline-flex items-center gap-1 rounded-md border border-border bg-background px-1.5 py-0.5 text-xs text-foreground hover:bg-panel-muted disabled:opacity-50"
                      type="button"
                      title="Marquer comme version B"
                      onclick={() => (selectedB = v.id)}
                    >B</button>
                    <button
                      class="inline-flex items-center gap-1 rounded-md border border-accent bg-accent px-2 py-0.5 text-xs font-medium text-accent-foreground hover:bg-accent-hover disabled:opacity-50"
                      type="button"
                      disabled={restoring === v.id}
                      onclick={() => restore(v.id)}
                    >
                      <RotateCcw size={10} strokeWidth={2.5} aria-hidden="true" />
                      {restoring === v.id ? 'Restauration…' : 'Restaurer'}
                    </button>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </aside>

        <section class="flex min-h-0 flex-col">
          <header class="flex items-center gap-2 border-b border-border bg-background px-3 py-2 text-xs text-subtle">
            <GitCompare size={12} strokeWidth={2} aria-hidden="true" />
            <span>A → B : diff unifié</span>
            {#if diffLoading}
              <Loader2 size={11} strokeWidth={2} class="animate-spin" aria-hidden="true" />
            {/if}
          </header>
          <pre
            class="m-0 flex-1 overflow-auto whitespace-pre-wrap break-words bg-background p-3 font-mono text-xs text-foreground"
          >{diffContent || (selectedA && selectedB ? 'Chargement…' : 'Sélectionnez deux versions (clic pour A, Maj+clic pour B).')}</pre>
        </section>
      </div>
    </div>
  </div>
{/if}