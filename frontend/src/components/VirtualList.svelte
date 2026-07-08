<script lang="ts" generics="T">
  import { onMount, untrack, type Snippet } from 'svelte';

  type Props = {
    items: T[];
    itemHeight: number;
    overscan?: number;
    class?: string;
    ariaLabel?: string;
    children: Snippet<[T, number]>;
  };

  let {
    items,
    itemHeight,
    overscan = 4,
    class: klass = '',
    ariaLabel,
    children
  }: Props = $props();

  let viewport: HTMLDivElement | undefined = $state();
  let viewportHeight = $state(0);
  let scrollTop = $state(0);

  const totalHeight = $derived(items.length * itemHeight);

  const visibleRange = $derived.by(() => {
    if (viewportHeight <= 0) return { start: 0, end: 0 };
    const start = Math.max(0, Math.floor(scrollTop / itemHeight) - overscan);
    const end = Math.min(
      items.length,
      Math.ceil((scrollTop + viewportHeight) / itemHeight) + overscan
    );
    return { start, end };
  });

  const visibleItems = $derived(
    items.slice(visibleRange.start, visibleRange.end).map((item, i) => ({
      item,
      index: visibleRange.start + i,
      top: (visibleRange.start + i) * itemHeight
    }))
  );

  function onScroll(event: Event): void {
    scrollTop = (event.currentTarget as HTMLDivElement).scrollTop;
  }

  onMount(() => {
    if (!viewport) return;
    const ro = new ResizeObserver((entries) => {
      const entry = entries[0];
      if (!entry) return;
      viewportHeight = entry.contentRect.height;
    });
    ro.observe(viewport);
    viewportHeight = viewport.clientHeight;
    return () => ro.disconnect();
  });

  $effect(() => {
    untrack(() => {
      if (!viewport) return;
      if (scrollTop + viewportHeight > totalHeight) {
        viewport.scrollTop = Math.max(0, totalHeight - viewportHeight);
      }
    });
  });
</script>

<div
  bind:this={viewport}
  onscroll={onScroll}
  class="overflow-y-auto {klass}"
  role="listbox"
  aria-label={ariaLabel}
  tabindex="0"
>
  <div style="position: relative; height: {totalHeight}px;">
    {#each visibleItems as entry (entry.index)}
      <div
        style="position: absolute; top: {entry.top}px; left: 0; right: 0; height: {itemHeight}px;"
        role="option"
        aria-selected={false}
      >
        {@render children(entry.item, entry.index)}
      </div>
    {/each}
  </div>
</div>