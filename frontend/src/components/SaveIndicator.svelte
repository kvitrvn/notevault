<script context="module" lang="ts">
  export type SaveState = 'clean' | 'dirty' | 'saving' | 'error';
</script>

<script lang="ts">
  export let state: SaveState = 'clean';
  export let lastSavedAt: Date | null = null;
  export let compact = false;

  const labels: Record<SaveState, string> = {
    clean: 'Enregistré',
    dirty: 'Modifié',
    saving: 'Enregistrement…',
    error: 'Erreur d’écriture'
  };

  const colors: Record<SaveState, string> = {
    clean: 'var(--color-accent)',
    dirty: 'var(--color-faint)',
    saving: 'var(--color-faint)',
    error: 'var(--color-danger)'
  };
</script>

<span
  class="inline-flex items-center gap-1.5 text-xs"
  aria-live="polite"
  aria-atomic="true"
>
  <span
    class="save-dot inline-block h-1.5 w-1.5 rounded-full"
    style="background-color: {colors[state]}; {state === 'saving' ? 'animation: pulse 1s ease-in-out infinite;' : ''}"
    aria-hidden="true"
  ></span>
  {#if compact}
    <span class="sr-only">{labels[state]}</span>
  {:else}
    <span class="text-faint">
      {#if state === 'clean' && lastSavedAt}
        {labels[state]} · {lastSavedAt.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' })}
      {:else}
        {labels[state]}
      {/if}
    </span>
  {/if}
</span>

<style>
  @keyframes pulse {
    0%, 100% { opacity: 0.4; }
    50% { opacity: 1; }
  }
</style>