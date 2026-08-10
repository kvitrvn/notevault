<script lang="ts">
  type Props = {
    open: boolean;
    path: string;
    onTakeDisk: () => void;
    onForceSave: () => void;
    onClose: () => void;
  };

  let { open, path, onTakeDisk, onForceSave, onClose }: Props = $props();
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="conflict-title"
    onclick={onClose}
    onkeydown={(e) => {
      if (e.key === 'Escape') onClose();
    }}
    tabindex="-1"
  >
    <div
      class="w-full max-w-md rounded-lg border border-border bg-panel p-6 shadow-xl"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      role="document"
    >
      <h2 id="conflict-title" class="text-lg font-semibold text-foreground">
        Conflit d'édition
      </h2>
      <p class="mt-3 text-sm text-subtle">
        Le fichier <code class="rounded bg-panel-muted px-1 text-foreground">{path}</code>
        a été modifié sur disque alors que cette note contient des modifications locales non
        enregistrées. Choisissez une résolution :
      </p>
      <ul class="mt-4 space-y-3 text-sm text-subtle">
        <li>
          <strong class="text-foreground">Recharger depuis disque</strong> — abandonne vos
          modifications locales et prend la version actuelle du fichier.
        </li>
        <li>
          <strong class="text-foreground">Forcer l'enregistrement</strong> — écrase la version
          disque avec votre version locale (la version disque actuelle sera conservée dans
          l'historique de la note).
        </li>
      </ul>
      <div class="mt-6 flex justify-end gap-2">
        <button
          type="button"
          class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
          onclick={onClose}
        >
          Plus tard
        </button>
        <button
          type="button"
          class="rounded-md border border-border bg-panel-muted px-3 py-1.5 text-sm text-foreground hover:border-accent hover:text-accent"
          onclick={onTakeDisk}
        >
          Recharger depuis disque
        </button>
        <button
          type="button"
          class="rounded-md bg-accent px-3 py-1.5 text-sm font-medium text-foreground hover:opacity-90"
          onclick={onForceSave}
        >
          Forcer l'enregistrement
        </button>
      </div>
    </div>
  </div>
{/if}
