<script lang="ts">
  import AlertTriangle from '@lucide/svelte/icons/alert-triangle';
  import Undo2 from '@lucide/svelte/icons/undo-2';
  import X from '@lucide/svelte/icons/x';
  import type { vault } from '../../wailsjs/go/models';

  type RecoverySnapshot = vault.RecoverySnapshot;

  type Props = {
    open: boolean;
    snapshot: RecoverySnapshot | null;
    onRecover: (buffer: string) => void;
    onDiscard: () => void;
    onClose: () => void;
  };

  let { open, snapshot, onRecover, onDiscard, onClose }: Props = $props();

  function formatDate(value: unknown): string {
    if (!value) return '';
    const date = new Date(String(value));
    return Number.isNaN(date.getTime())
      ? ''
      : date.toLocaleString('fr-FR', { dateStyle: 'short', timeStyle: 'short' });
  }

  const preview = $derived.by(() => {
    if (!snapshot) return '';
    const buf = snapshot.buffer ?? '';
    return buf.length > 200 ? buf.slice(0, 200) + '…' : buf;
  });

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open && snapshot?.hasRecovery}
  <div
    class="fixed inset-0 z-[58] grid place-items-center px-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="recovery-title"
  >
    <button
      type="button"
      class="absolute inset-0 bg-black/60"
      aria-label="Fermer la récupération"
      onclick={onClose}
    ></button>
    <div
      class="relative w-full max-w-md overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center justify-between border-b border-border bg-background px-4 py-3">
        <h2 id="recovery-title" class="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <AlertTriangle size={15} strokeWidth={2} class="text-accent" aria-hidden="true" />
          Récupération d’une note
        </h2>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
          aria-label="Fermer"
          onclick={onClose}
        >
          <X size={13} strokeWidth={2} aria-hidden="true" />
        </button>
      </header>

      <div class="flex flex-col gap-3 px-4 py-4 text-sm">
        <p class="leading-6 text-subtle">
          Une sauvegarde automatique d’une note modifiée a été trouvée. Le
          fichier sur disque n’a pas été touché depuis.
        </p>
        <div class="rounded-md border border-border bg-background px-3 py-2 text-xs text-faint">
          <p>
            <span class="text-subtle">Note :</span>
            <span class="ml-1 break-all text-foreground">{snapshot.notePath}</span>
          </p>
          <p class="mt-1">
            <span class="text-subtle">Sauvegardé le :</span>
            <span class="ml-1 text-foreground">{formatDate(snapshot.bufferSavedAt)}</span>
          </p>
        </div>
        {#if preview}
          <pre class="max-h-40 overflow-y-auto whitespace-pre-wrap rounded-md border border-border bg-background p-3 font-mono text-[0.7rem] text-foreground">{preview}</pre>
        {/if}
        <p class="text-xs text-faint">
          Récupérer remplace le contenu actuel de la note par cette version.
          Vous pourrez toujours annuler via l’historique local (Ctrl+Shift+H).
        </p>
      </div>

      <footer class="flex items-center justify-end gap-2 border-t border-border bg-background px-4 py-3">
        <button
          type="button"
          class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
          onclick={onDiscard}
        >
          Ignorer
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
          onclick={() => onRecover(snapshot.buffer)}
        >
          <Undo2 size={13} strokeWidth={2} aria-hidden="true" />
          Récupérer
        </button>
      </footer>
    </div>
  </div>
{/if}
