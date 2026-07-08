<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil';
  import FolderInput from '@lucide/svelte/icons/folder-input';
  import Copy from '@lucide/svelte/icons/copy';
  import CopyPlus from '@lucide/svelte/icons/copy-plus';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import Trash2 from '@lucide/svelte/icons/trash-2';

  type Item = {
    label: string;
    icon?: typeof Pencil;
    danger?: boolean;
    onPick: () => void;
  };

  type Props = {
    open: boolean;
    x: number;
    y: number;
    items: Item[];
    onClose: () => void;
  };

  let { open, x, y, items, onClose }: Props = $props();

  let menuEl: HTMLDivElement | undefined = $state();
  let posX = $state(0);
  let posY = $state(0);

  $effect(() => {
    if (!open) return;
    posX = x;
    posY = y;
    requestAnimationFrame(() => {
      if (!menuEl) return;
      const rect = menuEl.getBoundingClientRect();
      const padding = 8;
      const maxX = window.innerWidth - rect.width - padding;
      const maxY = window.innerHeight - rect.height - padding;
      if (posX > maxX) posX = maxX;
      if (posY > maxY) posY = maxY;
      if (posX < padding) posX = padding;
      if (posY < padding) posY = padding;
    });
  });

  function onWindowKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    }
  }

  function onWindowClick(event: MouseEvent): void {
    if (!open) return;
    const target = event.target as HTMLElement | null;
    if (target && menuEl && menuEl.contains(target)) return;
    onClose();
  }
</script>

<svelte:window onkeydown={onWindowKey} onclick={onWindowClick} />

{#if open}
  <div
    bind:this={menuEl}
    class="fixed z-50 min-w-[12rem] rounded-md border border-border bg-panel py-1 text-sm shadow-xl"
    style="left: {posX}px; top: {posY}px"
    role="menu"
    aria-label="Actions sur la note"
  >
    {#each items as item (item.label)}
      {@const Icon = item.icon}
      <button
        type="button"
        class={item.danger
          ? 'flex w-full items-center gap-2 px-3 py-1.5 text-left text-danger hover:bg-danger/10'
          : 'flex w-full items-center gap-2 px-3 py-1.5 text-left text-foreground hover:bg-panel-muted'}
        role="menuitem"
        onclick={() => {
          item.onPick();
          onClose();
        }}
      >
        {#if Icon}
          <Icon size={14} strokeWidth={2} aria-hidden="true" />
        {/if}
        <span>{item.label}</span>
      </button>
    {/each}
  </div>
{/if}