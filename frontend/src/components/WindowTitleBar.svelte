<script lang="ts">
  import Maximize from '@lucide/svelte/icons/maximize';
  import Minimize from '@lucide/svelte/icons/minimize';
  import Minus from '@lucide/svelte/icons/minus';
  import X from '@lucide/svelte/icons/x';
  import {
    Quit,
    WindowIsMaximised,
    WindowMinimise,
    WindowToggleMaximise
  } from '../../wailsjs/runtime/runtime';

  type Props = {
    onClose: () => Promise<boolean>;
  };

  let { onClose }: Props = $props();
  let maximised = $state(false);
  let closing = $state(false);

  async function refreshMaximised(): Promise<void> {
    try {
      maximised = await WindowIsMaximised();
    } catch {
      maximised = false;
    }
  }

  async function toggleMaximise(): Promise<void> {
    WindowToggleMaximise();
    window.setTimeout(() => void refreshMaximised(), 80);
  }

  async function closeWindow(): Promise<void> {
    if (closing) return;
    closing = true;
    try {
      if (await onClose()) {
        Quit();
      }
    } finally {
      closing = false;
    }
  }

  $effect(() => {
    void refreshMaximised();
  });
</script>

<header
  class="relative grid h-9 shrink-0 select-none items-center border-b border-border bg-panel text-foreground"
  style="--wails-draggable: drag"
  aria-label="Barre de fenêtre"
>
  <div
    class="absolute inset-0 grid place-items-center px-28"
    style="--wails-draggable: drag"
    role="button"
    tabindex="0"
    aria-label={maximised ? 'Restaurer la fenêtre' : 'Agrandir la fenêtre'}
    ondblclick={() => toggleMaximise()}
    onkeydown={(event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        void toggleMaximise();
      }
    }}
  >
    <span class="truncate text-xs font-medium tracking-normal text-subtle">NoteVault</span>
  </div>

  <div class="relative z-10 ml-auto flex h-full items-center" style="--wails-draggable: no-drag">
    <button
      class="inline-flex h-9 w-11 items-center justify-center text-subtle hover:bg-panel-muted hover:text-foreground"
      type="button"
      title="Réduire"
      aria-label="Réduire"
      onclick={() => WindowMinimise()}
      ondblclick={(event) => event.stopPropagation()}
    >
      <Minus size={14} strokeWidth={2} aria-hidden="true" />
    </button>
    <button
      class="inline-flex h-9 w-11 items-center justify-center text-subtle hover:bg-panel-muted hover:text-foreground"
      type="button"
      title={maximised ? 'Restaurer' : 'Agrandir'}
      aria-label={maximised ? 'Restaurer' : 'Agrandir'}
      aria-pressed={maximised}
      onclick={() => toggleMaximise()}
      ondblclick={(event) => event.stopPropagation()}
    >
      {#if maximised}
        <Minimize size={13} strokeWidth={2} aria-hidden="true" />
      {:else}
        <Maximize size={13} strokeWidth={2} aria-hidden="true" />
      {/if}
    </button>
    <button
      class="inline-flex h-9 w-11 items-center justify-center text-subtle hover:bg-danger hover:text-background disabled:hover:bg-transparent disabled:hover:text-subtle"
      type="button"
      title="Fermer"
      aria-label="Fermer"
      disabled={closing}
      onclick={() => closeWindow()}
      ondblclick={(event) => event.stopPropagation()}
    >
      <X size={15} strokeWidth={2} aria-hidden="true" />
    </button>
  </div>
</header>
