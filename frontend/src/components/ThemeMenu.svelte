<script lang="ts">
  import Palette from '@lucide/svelte/icons/palette';
  import Check from '@lucide/svelte/icons/check';
  import Moon from '@lucide/svelte/icons/moon';
  import Sun from '@lucide/svelte/icons/sun';
  import { ListThemes } from '../../wailsjs/go/main/App';
  import type { vault } from '../../wailsjs/go/models';

  type Theme = vault.Theme;

  type Props = {
    active: string;
    onSelect: (id: string) => void;
  };

  let { active, onSelect }: Props = $props();

  let open = $state(false);
  let themes = $state<Theme[]>([]);
  let rootEl: HTMLDivElement | undefined = $state();

  $effect(() => {
    void ListThemes()
      .then((list) => {
        themes = (list ?? []) as Theme[];
      })
      .catch(() => {
        themes = [];
      });
  });

  function toggle(): void {
    open = !open;
  }

  function pick(id: string): void {
    onSelect(id);
    open = false;
  }

  function onWindowClick(event: MouseEvent): void {
    if (!open) return;
    const target = event.target as Node | null;
    if (rootEl && target && !rootEl.contains(target)) {
      open = false;
    }
  }

  function onWindowKey(event: KeyboardEvent): void {
    if (open && event.key === 'Escape') {
      event.preventDefault();
      open = false;
    }
  }

  $effect(() => {
    if (open) {
      window.addEventListener('mousedown', onWindowClick);
      window.addEventListener('keydown', onWindowKey);
      return () => {
        window.removeEventListener('mousedown', onWindowClick);
        window.removeEventListener('keydown', onWindowKey);
      };
    }
  });
</script>

<div class="relative" bind:this={rootEl}>
  <button
    type="button"
    class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
    aria-label="Thème"
    aria-haspopup="menu"
    aria-expanded={open}
    title="Thème"
    onclick={toggle}
  >
    {#if active === 'light'}
      <Sun size={15} strokeWidth={2} aria-hidden="true" />
    {:else}
      <Moon size={15} strokeWidth={2} aria-hidden="true" />
    {/if}
  </button>

  {#if open}
    <div
      class="absolute right-0 top-11 z-30 w-60 overflow-hidden rounded-lg border border-border bg-panel shadow-xl"
      role="menu"
    >
      <header class="flex items-center gap-1.5 border-b border-border bg-background px-3 py-2 text-xs font-semibold uppercase text-subtle">
        <Palette size={12} strokeWidth={2} aria-hidden="true" />
        Thèmes
      </header>
      <ul class="max-h-80 overflow-y-auto py-1">
        {#each themes as theme (theme.id)}
          <li>
            <button
              type="button"
              class="flex w-full items-center justify-between gap-2 px-3 py-1.5 text-left text-sm text-foreground hover:bg-panel-muted"
              role="menuitemradio"
              aria-checked={active === theme.id}
              onclick={() => pick(theme.id)}
            >
              <span class="flex items-center gap-2">
                <span
                  class="inline-block h-4 w-4 rounded-full border border-border-strong"
                  style="background: var(--color-accent, #7fc8ba);"
                  aria-hidden="true"
                ></span>
                {theme.name}
                {#if theme.builtin}
                  <span class="rounded bg-background px-1 text-[0.6rem] uppercase text-faint">intégré</span>
                {/if}
              </span>
              {#if active === theme.id}
                <Check size={12} strokeWidth={2.5} class="shrink-0 text-accent" aria-hidden="true" />
              {/if}
            </button>
          </li>
        {/each}
        {#if themes.length === 0}
          <li class="px-3 py-3 text-center text-xs text-subtle">Aucun thème.</li>
        {/if}
      </ul>
      <p class="border-t border-border bg-background px-3 py-2 text-[0.65rem] text-faint">
        Ajoutez vos thèmes dans
        <code class="rounded bg-panel px-1 py-0.5">.notevault/themes/</code>.
      </p>
    </div>
  {/if}
</div>
