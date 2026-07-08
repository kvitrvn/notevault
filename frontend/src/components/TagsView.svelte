<script lang="ts">
  import Hash from '@lucide/svelte/icons/hash';
  import X from '@lucide/svelte/icons/x';
  import ArrowUpDown from '@lucide/svelte/icons/arrow-up-down';

  type TagCount = { tag: string; count: number };

  type Props = {
    open: boolean;
    tags: TagCount[];
    onPick: (tag: string) => void;
    onClose: () => void;
  };

  let { open, tags, onPick, onClose }: Props = $props();

  type SortMode = 'alpha' | 'count';
  let sort: SortMode = $state('count');

  const sorted = $derived.by(() => {
    const arr = [...tags];
    if (sort === 'alpha') {
      arr.sort((a, b) => a.tag.localeCompare(b.tag));
    } else {
      arr.sort((a, b) => b.count - a.count || a.tag.localeCompare(b.tag));
    }
    return arr;
  });
</script>

<svelte:window
  onkeydown={(e) => {
    if (open && e.key === 'Escape') onClose();
  }}
/>

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[10vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Vue Tags"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer la vue Tags"
      onclick={onClose}
    ></button>
    <div
      class="relative flex max-h-[80vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <div class="min-w-0">
          <h2 class="flex items-center gap-1.5 text-base font-semibold text-foreground">
            <Hash size={16} strokeWidth={2} aria-hidden="true" />
            Tags
            <span class="ml-1 rounded-md border border-border-strong bg-background px-2 py-0.5 text-xs text-subtle">
              {tags.length}
            </span>
          </h2>
          <p class="mt-0.5 text-xs text-subtle">
            Cliquez sur un tag pour filtrer la sidebar. <kbd class="rounded border border-border-strong bg-background px-1">Esc</kbd> ferme.
          </p>
        </div>
        <div class="flex items-center gap-2">
          <div class="inline-flex items-center rounded-md border border-border bg-background p-0.5 text-xs">
            <button
              class={sort === 'count'
                ? 'inline-flex h-6 items-center gap-1 rounded px-2 font-medium text-foreground'
                : 'inline-flex h-6 items-center gap-1 rounded px-2 text-subtle hover:text-foreground'}
              type="button"
              aria-pressed={sort === 'count'}
              onclick={() => (sort = 'count')}
            >
              <ArrowUpDown size={11} strokeWidth={2} aria-hidden="true" />
              Fréquence
            </button>
            <button
              class={sort === 'alpha'
                ? 'inline-flex h-6 items-center gap-1 rounded px-2 font-medium text-foreground'
                : 'inline-flex h-6 items-center gap-1 rounded px-2 text-subtle hover:text-foreground'}
              type="button"
              aria-pressed={sort === 'alpha'}
              onclick={() => (sort = 'alpha')}
            >
              A → Z
            </button>
          </div>
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
      </div>

      <div class="flex-1 overflow-y-auto p-4">
        {#if tags.length === 0}
          <p class="text-sm text-subtle">
            Aucun tag n'est encore utilisé. Ajoutez des tags depuis l'éditeur.
          </p>
        {:else}
          <ul class="flex flex-wrap gap-2">
            {#each sorted as t (t.tag)}
              <li>
                <button
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-3 py-1 text-sm text-foreground hover:border-accent hover:bg-accent/10"
                  onclick={() => onPick(t.tag)}
                >
                  <Hash size={11} strokeWidth={2.5} class="text-accent" aria-hidden="true" />
                  <span>{t.tag}</span>
                  <span class="rounded-full bg-panel-muted px-1.5 py-0 text-xs text-subtle">
                    {t.count}
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    </div>
  </div>
{/if}