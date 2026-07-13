<script lang="ts">
  import { tick } from 'svelte';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import Plus from '@lucide/svelte/icons/plus';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import { SelectExistingVaultDirectory, SelectVaultParentDirectory } from '../../wailsjs/go/main/App';
  import { domain } from '../../wailsjs/go/models';
  import { finalVaultPath, validateVaultDraft } from '../lib/vault-manager';

  type Props = {
    open: boolean;
    embedded?: boolean;
    status: domain.ApplicationStatus | null;
    busy: boolean;
    error: string;
    onOpen: (path: string) => void;
    onCreate: (request: domain.CreateVaultRequest) => void;
    onForget: (path: string) => void;
    onClose: () => void;
  };

  let { open, embedded = false, status, busy, error, onOpen, onCreate, onForget, onClose }: Props = $props();
  let view = $state<'list' | 'create'>('list');
  let name = $state('');
  let parentPath = $state('');
  let encrypted = $state(false);
  let passphrase = $state('');
  let confirmation = $state('');
  let localError = $state('');
  let dialogEl: HTMLElement | undefined = $state();
  let nameInput: HTMLInputElement | undefined = $state();
  let previousFocus: HTMLElement | null = null;

  const previewPath = $derived(finalVaultPath(parentPath, name));

  $effect(() => {
    if (!open) return;
    view = 'list';
    localError = '';
    if (!embedded) previousFocus = document.activeElement as HTMLElement | null;
    void tick().then(() => {
      const target = dialogEl?.querySelector<HTMLElement>('button:not([disabled]), input:not([disabled])');
      target?.focus();
    });
    return () => {
      if (!embedded) previousFocus?.focus();
    };
  });

  async function browseExisting(): Promise<void> {
    localError = '';
    try {
      const path = await SelectExistingVaultDirectory();
      if (path) onOpen(path);
    } catch (err) {
      localError = String(err);
    }
  }

  async function browseParent(): Promise<void> {
    localError = '';
    try {
      const path = await SelectVaultParentDirectory();
      if (path) parentPath = path;
    } catch (err) {
      localError = String(err);
    }
  }

  function startCreate(): void {
    view = 'create';
    localError = '';
    void tick().then(() => nameInput?.focus());
  }

  function submitCreate(): void {
    const validation = validateVaultDraft({ name, parentPath, encrypted, passphrase, confirmation });
    if (validation) {
      localError = validation;
      return;
    }
    onCreate(domain.CreateVaultRequest.createFrom({ name: name.trim(), parentPath, encrypted, passphrase }));
  }

  function onKey(event: KeyboardEvent): void {
    if (!open || embedded) return;
    if (event.key === 'Escape' && !busy) {
      event.preventDefault();
      onClose();
      return;
    }
    if (event.key !== 'Tab' || !dialogEl) return;
    const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])'));
    if (focusable.length === 0) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class={embedded ? 'flex min-h-0 flex-1 items-start justify-center overflow-y-auto px-5 py-8' : 'fixed inset-0 z-50 grid place-items-center px-4'}
    role={embedded ? undefined : 'dialog'}
    aria-modal={embedded ? undefined : 'true'}
    aria-labelledby="vault-picker-title"
  >
    {#if !embedded}<button class="absolute inset-0 bg-black/60" type="button" aria-label="Fermer" onclick={onClose}></button>{/if}
    <section bind:this={dialogEl} class="relative w-full max-w-2xl overflow-hidden rounded-lg border border-border bg-panel shadow-lg">
      <header class="flex min-h-12 items-center gap-3 border-b border-border bg-background px-4 py-3">
        <FolderOpen size={17} strokeWidth={2} class="text-accent" aria-hidden="true" />
        <h1 id="vault-picker-title" class="text-base font-semibold text-foreground">Choisir un coffre</h1>
        {#if !embedded}
          <button type="button" class="ml-auto grid h-8 w-8 place-items-center rounded-md border border-border text-subtle hover:bg-panel-muted hover:text-foreground" aria-label="Fermer" onclick={onClose} disabled={busy}>
            <X size={14} aria-hidden="true" />
          </button>
        {/if}
      </header>

      {#if view === 'list'}
        <div class="px-4 py-4">
          {#if status?.recentVaults?.length}
            <ul class="divide-y divide-border border-y border-border" aria-label="Coffres récents">
              {#each status.recentVaults as recent (recent.path)}
                <li class="flex min-w-0 items-center gap-2 py-1.5">
                  <button type="button" class="min-w-0 flex-1 px-2 py-2 text-left hover:bg-panel-muted disabled:cursor-not-allowed" onclick={() => onOpen(recent.path)} disabled={busy || !recent.available}>
                    <span class="flex items-center gap-2 text-sm font-medium text-foreground">
                      <span class="truncate">{recent.name}</span>
                      {#if recent.encrypted}<span class="text-xs font-normal text-subtle">Chiffré</span>{/if}
                      {#if !recent.available}<span class="text-xs font-normal text-danger">Indisponible</span>{/if}
                    </span>
                    <span class="mt-0.5 block truncate text-xs text-subtle" title={recent.path}>{recent.path}</span>
                  </button>
                  <button type="button" class="grid h-8 w-8 shrink-0 place-items-center rounded-md text-subtle hover:bg-panel-muted hover:text-danger" aria-label={`Retirer ${recent.name} des récents`} title="Retirer des récents" onclick={() => onForget(recent.path)} disabled={busy || recent.active}>
                    <Trash2 size={14} aria-hidden="true" />
                  </button>
                </li>
              {/each}
            </ul>
          {:else}
            <p class="border-y border-border px-2 py-6 text-center text-sm text-subtle">Aucun coffre récent.</p>
          {/if}

          <div class="mt-4 flex flex-wrap justify-end gap-2">
            <button type="button" class="rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground hover:bg-panel-muted" onclick={() => void browseExisting()} disabled={busy}>
              Ouvrir un coffre existant
            </button>
            <button type="button" class="inline-flex items-center gap-1.5 rounded-md border border-accent bg-accent px-3 py-2 text-sm font-medium text-accent-foreground hover:bg-accent-hover" onclick={startCreate} disabled={busy}>
              <Plus size={14} aria-hidden="true" /> Créer un coffre
            </button>
          </div>
        </div>
      {:else}
        <form class="px-4 py-4" onsubmit={(event) => { event.preventDefault(); submitCreate(); }}>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="block text-sm font-medium text-foreground">
              Nom
              <input bind:this={nameInput} bind:value={name} class="mt-1.5 h-10 w-full rounded-md border border-border-strong bg-background px-3 text-sm" maxlength="80" autocomplete="off" disabled={busy} />
            </label>
            <div>
              <span class="block text-sm font-medium text-foreground">Emplacement</span>
              <div class="mt-1.5 flex gap-2">
                <input value={parentPath} class="h-10 min-w-0 flex-1 rounded-md border border-border-strong bg-background px-3 text-sm" readonly aria-label="Dossier parent" />
                <button type="button" class="rounded-md border border-border bg-background px-3 text-sm hover:bg-panel-muted" onclick={() => void browseParent()} disabled={busy}>Parcourir…</button>
              </div>
            </div>
          </div>

          <p class="mt-3 truncate border-l-2 border-border-strong pl-3 text-xs text-subtle" title={previewPath}>Chemin final : {previewPath || '—'}</p>

          <fieldset class="mt-4 border-t border-border pt-4">
            <legend class="text-sm font-medium text-foreground">Protection</legend>
            <div class="mt-2 grid gap-2 sm:grid-cols-2">
              <label class="flex cursor-pointer gap-2 rounded-md border border-border bg-background p-3 text-sm">
                <input type="radio" name="protection" checked={!encrypted} onchange={() => (encrypted = false)} disabled={busy} />
                <span><strong class="block font-medium">Markdown lisible</strong><span class="mt-0.5 block text-xs text-subtle">Compatible avec les autres éditeurs.</span></span>
              </label>
              <label class="flex cursor-pointer gap-2 rounded-md border border-border bg-background p-3 text-sm">
                <input type="radio" name="protection" checked={encrypted} onchange={() => (encrypted = true)} disabled={busy} />
                <span><strong class="flex items-center gap-1 font-medium"><ShieldCheck size={13} aria-hidden="true" /> Coffre chiffré</strong><span class="mt-0.5 block text-xs text-subtle">Phrase secrète irrécupérable.</span></span>
              </label>
            </div>
          </fieldset>

          {#if encrypted}
            <div class="mt-4 grid gap-4 sm:grid-cols-2">
              <label class="text-sm font-medium">Phrase secrète<input bind:value={passphrase} type="password" autocomplete="new-password" class="mt-1.5 h-10 w-full rounded-md border border-border-strong bg-background px-3" disabled={busy} /></label>
              <label class="text-sm font-medium">Confirmer<input bind:value={confirmation} type="password" autocomplete="new-password" class="mt-1.5 h-10 w-full rounded-md border border-border-strong bg-background px-3" disabled={busy} /></label>
            </div>
            <p class="mt-2 text-xs text-danger">Conservez cette phrase secrète : NoteVault ne peut pas la récupérer.</p>
          {/if}

          <div class="mt-5 flex justify-end gap-2 border-t border-border pt-3">
            <button type="button" class="rounded-md border border-border px-3 py-2 text-sm text-subtle hover:bg-panel-muted" onclick={() => (view = 'list')} disabled={busy}>Retour</button>
            <button type="submit" class="rounded-md border border-accent bg-accent px-3 py-2 text-sm font-medium text-accent-foreground hover:bg-accent-hover" disabled={busy}>Créer et ouvrir</button>
          </div>
        </form>
      {/if}

      <div class="min-h-10 border-t border-border bg-background px-4 py-2 text-sm text-danger" role="status" aria-live="polite">
        {busy ? 'Préparation et indexation du coffre…' : localError || error}
      </div>
    </section>
  </div>
{/if}
