<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import CornerDownLeft from '@lucide/svelte/icons/corner-down-left';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import type { Snippet } from 'svelte';

  type Entry = {
    relativePath: string;
    title: string;
    updatedAt: string;
    score: number;
  };

  type Props = {
    open: boolean;
    entries: Entry[];
    onPick: (entry: Entry) => void;
    onClose: () => void;
    footer?: Snippet;
  };

  let { open, entries, onPick, onClose, footer }: Props = $props();

  let inputEl: HTMLInputElement | undefined = $state();
  let query = $state('');
  let selectedIndex = $state(0);
  const MAX_RESULTS = 50;

  const filtered = $derived.by(() => {
    const q = query.trim();
    if (!q) {
      return entries.slice(0, MAX_RESULTS).map((e) => ({ ...e, score: 0 }));
    }
    return scoreEntries(entries, q).slice(0, MAX_RESULTS);
  });

  $effect(() => {
    if (open) {
      query = '';
      selectedIndex = 0;
      requestAnimationFrame(() => inputEl?.focus());
    }
  });

  $effect(() => {
    if (selectedIndex >= filtered.length) {
      selectedIndex = Math.max(0, filtered.length - 1);
    }
  });

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (filtered.length > 0) {
        selectedIndex = (selectedIndex + 1) % filtered.length;
      }
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      if (filtered.length > 0) {
        selectedIndex = (selectedIndex - 1 + filtered.length) % filtered.length;
      }
    } else if (event.key === 'Enter') {
      event.preventDefault();
      const entry = filtered[selectedIndex];
      if (entry) onPick(entry);
    }
  }

  function scoreEntries(entries: Entry[], raw: string): Entry[] {
    const q = raw.toLowerCase();
    const words = q.split(/\s+/).filter(Boolean);
    const out: Entry[] = [];
    for (const e of entries) {
      const title = e.title.toLowerCase();
      const path = e.relativePath.toLowerCase();
      let s = 0;
      let matched = 0;
      for (const w of words) {
        const tScore = subsequenceScore(title, w) * 2;
        const pScore = subsequenceScore(path, w);
        const local = Math.max(tScore, pScore);
        if (local > 0) {
          s += local;
          matched++;
        }
      }
      if (matched === words.length && s > 0) {
        out.push({ ...e, score: s });
      }
    }
    out.sort((a, b) => b.score - a.score);
    return out;
  }

  function subsequenceScore(haystack: string, needle: string): number {
    if (!needle) return 0;
    let hi = 0;
    let ni = 0;
    let score = 0;
    let lastMatch = -1;
    while (hi < haystack.length && ni < needle.length) {
      if (haystack[hi] === needle[ni]) {
        if (lastMatch >= 0) {
          score += 1 / (hi - lastMatch);
        } else {
          score += 1;
        }
        lastMatch = hi;
        ni++;
      }
      hi++;
    }
    if (ni < needle.length) return 0;
    if (haystack.startsWith(needle)) score += 2;
    return score;
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[12vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Recherche rapide"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer la recherche"
      onclick={onClose}
    ></button>
    <div
      class="relative w-full max-w-xl overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center gap-2 border-b border-border px-3">
        <Search size={16} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
        <input
          bind:this={inputEl}
          bind:value={query}
          type="search"
          class="block w-full bg-transparent py-3 text-sm text-foreground outline-none placeholder:text-faint"
          placeholder="Rechercher une note, un tag, un chemin…"
          aria-label="Requête de recherche"
          spellcheck="false"
          autocomplete="off"
        />
        <kbd
          class="shrink-0 rounded border border-border bg-background px-1.5 py-0.5 text-xs text-subtle"
          >esc</kbd
        >
      </div>

      <ul
        class="max-h-[50vh] overflow-y-auto py-1"
        role="listbox"
        aria-label="Résultats"
      >
        {#if filtered.length === 0}
          <li class="px-3 py-4 text-center text-sm text-subtle">
            {#if entries.length === 0}
              Aucune note indexée.
            {:else}
              Aucune note ne correspond à « {query} ».
            {/if}
          </li>
        {:else}
          {#each filtered as entry, idx (entry.relativePath)}
            <li role="option" aria-selected={idx === selectedIndex}>
              <button
                type="button"
                class={idx === selectedIndex
                  ? 'flex w-full items-center justify-between gap-2 bg-accent/15 px-3 py-2 text-left text-foreground'
                  : 'flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-foreground hover:bg-panel-muted'}
                onmouseenter={() => (selectedIndex = idx)}
                onclick={() => onPick(entry)}
              >
                <span class="flex min-w-0 flex-col">
                  <span class="truncate text-sm font-medium">
                    {entry.title || 'Sans titre'}
                  </span>
                  <span class="truncate text-xs text-subtle">{entry.relativePath}</span>
                </span>
                {#if idx === selectedIndex}
                  <CornerDownLeft
                    size={14}
                    strokeWidth={2}
                    class="shrink-0 text-accent"
                    aria-hidden="true"
                  />
                {/if}
              </button>
            </li>
          {/each}
        {/if}
      </ul>

      <div
        class="flex items-center justify-between gap-3 border-t border-border bg-background px-3 py-1.5 text-xs text-subtle"
      >
        <div class="flex items-center gap-3">
          <span class="inline-flex items-center gap-1">
            <ArrowUp size={11} strokeWidth={2} aria-hidden="true" />
            <ArrowDown size={11} strokeWidth={2} aria-hidden="true" />
            naviguer
          </span>
          <span class="inline-flex items-center gap-1">
            <CornerDownLeft size={11} strokeWidth={2} aria-hidden="true" />
            ouvrir
          </span>
        </div>
        {#if footer}
          {@render footer()}
        {/if}
      </div>
    </div>
  </div>
{/if}