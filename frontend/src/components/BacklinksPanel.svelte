<script lang="ts">
  import Link2 from '@lucide/svelte/icons/link-2';
  import Loader2 from '@lucide/svelte/icons/loader';

  type NoteSummary = { relativePath: string; title: string; updatedAt: string };

  type Props = {
    title: string;
    excludePath: string;
    onOpen: (relativePath: string) => void;
  };

  let { title, excludePath, onOpen }: Props = $props();

  let loading = $state(false);
  let entries: NoteSummary[] = $state([]);
  let lastLoaded = '';

  $effect(() => {
    if (!title) {
      entries = [];
      lastLoaded = '';
      return;
    }
    const key = `${title}|${excludePath}`;
    if (key === lastLoaded) return;
    lastLoaded = key;
    loading = true;
    void load(title, excludePath);
  });

  async function load(t: string, exclude: string): Promise<void> {
    try {
      const { GetBacklinks } = await import('../../wailsjs/go/main/App');
      const res = await GetBacklinks(t, exclude, 50);
      entries = res ?? [];
    } catch (err) {
      console.error('GetBacklinks', err);
      entries = [];
    } finally {
      loading = false;
    }
  }
</script>

<section
  class="flex flex-col gap-1 border-t border-border bg-background px-4 py-2 text-xs text-subtle"
  aria-label="Backlinks"
>
  <header class="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-subtle">
    <Link2 size={11} strokeWidth={2.5} aria-hidden="true" />
    Backlinks
    {#if loading}
      <Loader2 size={11} strokeWidth={2} class="animate-spin" aria-hidden="true" />
    {:else}
      <span class="rounded-md border border-border-strong bg-panel px-1.5 py-0 text-faint">
        {entries.length}
      </span>
    {/if}
  </header>

  {#if !title}
    <p class="text-xs text-faint">Aucun titre à analyser.</p>
  {:else if entries.length === 0 && !loading}
    <p class="text-xs text-faint">Aucune note ne contient de lien vers « {title} ».</p>
  {:else}
    <ul class="flex flex-col gap-0.5">
      {#each entries as entry (entry.relativePath)}
        <li>
          <button
            type="button"
            class="-mx-1 inline-flex w-[calc(100%+0.5rem)] items-center gap-1.5 truncate rounded-md px-1 py-0.5 text-left text-foreground hover:bg-panel-muted"
            onclick={() => onOpen(entry.relativePath)}
            title={entry.relativePath}
          >
            <Link2 size={10} strokeWidth={2.5} class="shrink-0 text-accent" aria-hidden="true" />
            <span class="truncate text-xs">{entry.title || entry.relativePath}</span>
            <span class="ml-auto truncate text-faint">{entry.relativePath}</span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</section>
