<script lang="ts">
  import Hash from '@lucide/svelte/icons/hash';
  import X from '@lucide/svelte/icons/x';
  import Plus from '@lucide/svelte/icons/plus';

  type TagCount = { tag: string; count: number };

  type Props = {
    tags: string[];
    knownTags: TagCount[];
    onChange: (next: string[]) => void;
  };

  let { tags, knownTags, onChange }: Props = $props();

  let inputEl: HTMLInputElement | undefined = $state();
  let draft = $state('');
  let open = $state(false);
  let activeIndex = $state(0);

  const knownMap = $derived(new Map(knownTags.map((t) => [t.tag, t.count])));
  const suggestions = $derived.by(() => {
    const q = draft.trim().replace(/^#/, '').toLowerCase();
    if (!q) {
      return knownTags
        .filter((t) => !tags.includes(t.tag))
        .slice(0, 8);
    }
    return knownTags
      .filter((t) => !tags.includes(t.tag) && t.tag.toLowerCase().includes(q))
      .sort((a, b) => {
        const ax = a.tag.toLowerCase().startsWith(q) ? 0 : 1;
        const bx = b.tag.toLowerCase().startsWith(q) ? 0 : 1;
        if (ax !== bx) return ax - bx;
        return b.count - a.count;
      })
      .slice(0, 8);
  });

  $effect(() => {
    if (activeIndex >= suggestions.length) {
      activeIndex = Math.max(0, suggestions.length - 1);
    }
  });

  function commit(value: string): void {
    const v = value.trim().replace(/^#/, '').toLowerCase();
    if (!v) return;
    if (tags.includes(v)) {
      draft = '';
      return;
    }
    onChange([...tags, v]);
    draft = '';
    activeIndex = 0;
  }

  function commitDraft(): void {
    commit(draft);
  }

  function pickSuggestion(s: TagCount): void {
    commit(s.tag);
  }

  function removeAt(i: number): void {
    const next = tags.filter((_, idx) => idx !== i);
    onChange(next);
  }

  function onKey(event: KeyboardEvent): void {
    if (event.key === 'Enter' || event.key === ',') {
      event.preventDefault();
      if (suggestions.length > 0 && open && activeIndex < suggestions.length) {
        pickSuggestion(suggestions[activeIndex]);
      } else {
        commitDraft();
      }
      open = false;
    } else if (event.key === 'Backspace' && draft === '' && tags.length > 0) {
      event.preventDefault();
      removeAt(tags.length - 1);
    } else if (event.key === 'ArrowDown' && open) {
      event.preventDefault();
      activeIndex = (activeIndex + 1) % Math.max(1, suggestions.length);
    } else if (event.key === 'ArrowUp' && open) {
      event.preventDefault();
      activeIndex =
        (activeIndex - 1 + Math.max(1, suggestions.length)) %
        Math.max(1, suggestions.length);
    } else if (event.key === 'Escape') {
      open = false;
    }
  }

  function onFocus(): void {
    open = true;
  }

  function onBlur(): void {
    // Délai pour permettre le clic sur une suggestion.
    setTimeout(() => (open = false), 120);
  }
</script>

<div class="flex flex-wrap items-center gap-1.5">
  <span class="inline-flex items-center gap-1 text-xs text-subtle">
    <Hash size={12} strokeWidth={2} aria-hidden="true" />
    Tags
  </span>
  {#each tags as tag, i (tag + i)}
    <span
      class="inline-flex items-center gap-1 rounded-full border border-accent/40 bg-accent/10 px-2 py-0.5 text-xs text-accent"
    >
      {tag}
      <button
        type="button"
        class="inline-flex h-4 w-4 items-center justify-center rounded text-accent/80 hover:text-danger"
        title="Retirer ce tag"
        aria-label="Retirer le tag {tag}"
        onclick={() => removeAt(i)}
      >
        <X size={10} strokeWidth={2.5} aria-hidden="true" />
      </button>
    </span>
  {/each}
  <div class="relative">
    <input
      bind:this={inputEl}
      type="text"
      bind:value={draft}
      onkeydown={onKey}
      onfocus={onFocus}
      onblur={onBlur}
      placeholder="ajouter un tag…"
      class="w-32 rounded-md border border-border bg-background px-2 py-1 text-xs text-foreground outline-none focus:border-accent"
      spellcheck="false"
      autocomplete="off"
      aria-label="Nouveau tag"
    />
    {#if open && suggestions.length > 0}
      <ul
        class="absolute left-0 top-full z-20 mt-1 max-h-56 w-48 overflow-y-auto rounded-md border border-border bg-panel py-1 text-xs shadow-lg"
        role="listbox"
      >
        {#each suggestions as s, idx (s.tag)}
          <li>
            <button
              type="button"
              class={idx === activeIndex
                ? 'flex w-full items-center justify-between px-2 py-1 text-left text-foreground bg-accent/15'
                : 'flex w-full items-center justify-between px-2 py-1 text-left text-foreground hover:bg-panel-muted'}
              role="option"
              aria-selected={idx === activeIndex}
              onmousedown={(e) => e.preventDefault()}
              onclick={() => pickSuggestion(s)}
            >
              <span class="truncate">{s.tag}</span>
              <span class="ml-2 text-faint">{s.count}</span>
            </button>
          </li>
        {/each}
        {#if draft.trim() && !knownMap.has(draft.trim().replace(/^#/, '').toLowerCase())}
          <li>
            <button
              type="button"
              class="flex w-full items-center gap-1 px-2 py-1 text-left text-accent hover:bg-panel-muted"
              onmousedown={(e) => e.preventDefault()}
              onclick={commitDraft}
            >
              <Plus size={11} strokeWidth={2.5} aria-hidden="true" />
              Créer « {draft.trim().replace(/^#/, '').toLowerCase()} »
            </button>
          </li>
        {/if}
      </ul>
    {/if}
  </div>
</div>