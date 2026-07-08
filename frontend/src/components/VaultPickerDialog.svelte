<script lang="ts">
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import X from '@lucide/svelte/icons/x';
  import Cloud from '@lucide/svelte/icons/cloud';

  type Props = {
    open: boolean;
    initialPath: string;
    onPick: (path: string, options: { importFromDefault: boolean }) => void;
    onClose: () => void;
  };

  let { open, initialPath, onPick, onClose }: Props = $props();

  let path = $state(initialPath);
  let importDefault = $state(false);

  $effect(() => {
    if (open) {
      path = initialPath;
      importDefault = false;
    }
  });

  function commit(): void {
    const v = path.trim();
    if (!v) return;
    onPick(v, { importFromDefault: importDefault });
  }

  function isSyncCandidate(p: string): boolean {
    const norm = p.toLowerCase();
    return (
      norm.includes('dropbox') ||
      norm.includes('icloud') ||
      norm.includes('syncthing') ||
      norm.includes('/cloud/') ||
      norm.includes('onedrive')
    );
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      onClose();
    } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      commit();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[12vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Choisir un coffre"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative w-full max-w-md overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <h2 class="flex items-center gap-1.5 text-base font-semibold text-foreground">
          <FolderOpen size={16} strokeWidth={2} aria-hidden="true" />
          Choisir un coffre
        </h2>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          aria-label="Fermer"
          onclick={onClose}
        >
          <X size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>

      <div class="flex flex-col gap-3 px-4 py-3">
        <p class="text-xs text-subtle">
          Indiquez le dossier qui contient vos notes (un sous-dossier
          <code class="rounded bg-background px-1.5 py-0.5">notes/</code>
          sera créé s'il n'existe pas).
        </p>
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-subtle">Chemin du coffre</span>
          <input
            type="text"
            bind:value={path}
            placeholder="/chemin/du/coffre"
            class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
            spellcheck="false"
            autocomplete="off"
            onkeydown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault();
                commit();
              }
            }}
          />
        </label>

        {#if isSyncCandidate(path)}
          <p
            class="flex items-center gap-1.5 rounded-md border border-info/40 bg-info/10 px-2 py-1.5 text-xs text-info"
          >
            <Cloud size={12} strokeWidth={2} aria-hidden="true" />
            Ce dossier semble synchronisé (Dropbox / iCloud Drive / Syncthing / OneDrive).
            Aucun code de sync n'est inclus : vos notes seront juste stockées
            à cet endroit.
          </p>
        {/if}

        <label class="flex items-start gap-2 text-xs text-subtle">
          <input
            type="checkbox"
            bind:checked={importDefault}
            class="mt-0.5 h-4 w-4 rounded border-border bg-background accent-accent"
          />
          <span>
            Importer les notes depuis le coffre par défaut
            (<code class="rounded bg-background px-1.5 py-0.5">~/NoteVault</code>).
          </span>
        </label>
      </div>

      <div class="flex items-center justify-between gap-2 border-t border-border bg-background px-4 py-2.5 text-xs text-subtle">
        <span><kbd class="rounded border border-border-strong bg-panel px-1.5 py-0.5">Ctrl+Enter</kbd> pour valider</span>
        <div class="flex items-center gap-2">
          <button
            class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
            type="button"
            onclick={onClose}
          >
            Annuler
          </button>
          <button
            class="inline-flex items-center gap-2 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
            type="button"
            onclick={commit}
          >
            <FolderOpen size={13} strokeWidth={2} aria-hidden="true" />
            Ouvrir
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}