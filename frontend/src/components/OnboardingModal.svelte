<script lang="ts">
  import { tick } from 'svelte';
  import Check from '@lucide/svelte/icons/check';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Keyboard from '@lucide/svelte/icons/keyboard';
  import X from '@lucide/svelte/icons/x';
  import { SetOnboardingDismissed } from '../../wailsjs/go/main/App';

  type ThemeChoice = 'dark' | 'light';
  type Props = {
    open: boolean;
    initialTheme: ThemeChoice;
    initiallyDismissed?: boolean;
    onDone: (dismissed: boolean) => void;
  };

  let { open, initialTheme, initiallyDismissed = false, onDone }: Props = $props();
  let step = $state(0);
  let theme = $state<ThemeChoice>('dark');
  let dontShowAutomatically = $state(false);
  let busy = $state(false);
  let feedback = $state('');
  let dialogEl: HTMLElement | undefined = $state();
  let previousFocus: HTMLElement | null = null;
  const totalSteps = 3;

  const shortcuts = [
    ['Ctrl+P', 'Recherche rapide'],
    ['Ctrl+N', 'Nouvelle note'],
    ['Ctrl+S', 'Enregistrer'],
    ['Ctrl+Shift+D', 'Note du jour'],
    ['Ctrl+Shift+H', 'Historique'],
    ['Ctrl+/', 'Aide et raccourcis']
  ];

  $effect(() => {
    if (!open) return;
    previousFocus = document.activeElement as HTMLElement | null;
    step = 0;
    theme = initialTheme;
    dontShowAutomatically = initiallyDismissed;
    feedback = '';
    void tick().then(() => dialogEl?.querySelector<HTMLElement>('button, input')?.focus());
    return () => previousFocus?.focus();
  });

  $effect(() => {
    if (open) document.documentElement.dataset.theme = theme;
  });

  async function finish(): Promise<void> {
    busy = true;
    feedback = '';
    try {
      await SetOnboardingDismissed(dontShowAutomatically);
      window.localStorage.setItem('notevault-theme', theme);
      onDone(dontShowAutomatically);
    } catch (err) {
      feedback = String(err);
    } finally {
      busy = false;
    }
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape' && !busy) {
      event.preventDefault();
      void finish();
      return;
    }
    if (event.key !== 'Tab' || !dialogEl) return;
    const focusable = Array.from(dialogEl.querySelectorAll<HTMLElement>('button:not([disabled]), input:not([disabled])'));
    if (!focusable.length) return;
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
  <div class="fixed inset-0 z-[60] grid place-items-center px-4" role="dialog" aria-modal="true" aria-labelledby="guide-title">
    <div class="absolute inset-0 bg-black/65" aria-hidden="true"></div>
    <div bind:this={dialogEl} class="relative flex max-h-[calc(100vh-2rem)] w-full max-w-lg flex-col overflow-hidden rounded-lg border border-border bg-panel shadow-lg">
      <header class="flex items-center gap-3 border-b border-border bg-background px-4 py-3">
        <h2 id="guide-title" class="text-base font-semibold text-foreground">Guide NoteVault</h2>
        <span class="text-xs text-subtle">{step + 1} sur {totalSteps}</span>
        <button type="button" class="ml-auto grid h-8 w-8 place-items-center rounded-md border border-border text-subtle hover:bg-panel-muted hover:text-foreground" aria-label="Fermer le guide" onclick={() => void finish()} disabled={busy}>
          <X size={14} aria-hidden="true" />
        </button>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto px-5 py-5">
        {#if step === 0}
          <h3 class="text-lg font-semibold">Vos notes restent locales</h3>
          <p class="mt-2 text-sm leading-6 text-subtle">Un coffre Markdown reste lisible dans n’importe quel éditeur. Un coffre chiffré protège les notes et l’historique jusqu’au déverrouillage.</p>
          <ul class="mt-4 space-y-2 text-sm">
            <li class="flex gap-2"><Check size={14} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" /> Recherche, tags, dossiers et liens wiki.</li>
            <li class="flex gap-2"><Check size={14} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" /> Sauvegarde automatique, historique et récupération locale.</li>
            <li class="flex gap-2"><Check size={14} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" /> Changement de coffre depuis l’en-tête de la barre latérale.</li>
          </ul>
        {:else if step === 1}
          <h3 class="text-lg font-semibold">Apparence</h3>
          <p class="mt-2 text-sm text-subtle">Ce choix reste modifiable depuis l’en-tête de l’éditeur.</p>
          <div class="mt-4 grid grid-cols-2 gap-2">
            <button type="button" class={theme === 'dark' ? 'rounded-md border-2 border-accent bg-background px-4 py-3 text-sm' : 'rounded-md border border-border bg-background px-4 py-3 text-sm'} aria-pressed={theme === 'dark'} onclick={() => (theme = 'dark')}>Sombre</button>
            <button type="button" class={theme === 'light' ? 'rounded-md border-2 border-accent bg-background px-4 py-3 text-sm' : 'rounded-md border border-border bg-background px-4 py-3 text-sm'} aria-pressed={theme === 'light'} onclick={() => (theme = 'light')}>Clair</button>
          </div>
        {:else}
          <h3 class="flex items-center gap-2 text-lg font-semibold"><Keyboard size={17} class="text-accent" aria-hidden="true" /> Raccourcis essentiels</h3>
          <ul class="mt-4 divide-y divide-border border-y border-border">
            {#each shortcuts as shortcut (shortcut[0])}
              <li class="flex items-center justify-between gap-3 py-2 text-sm"><span>{shortcut[1]}</span><kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5 text-xs">{shortcut[0]}</kbd></li>
            {/each}
          </ul>
        {/if}
      </div>

      <footer class="border-t border-border bg-background px-4 py-3">
        <label class="flex items-center gap-2 text-sm text-subtle">
          <input type="checkbox" bind:checked={dontShowAutomatically} disabled={busy} />
          Ne plus afficher automatiquement
        </label>
        <div class="mt-3 flex items-center justify-between gap-2">
          <p class="min-h-5 text-xs text-danger" role="status" aria-live="polite">{feedback}</p>
          <div class="flex gap-2">
            {#if step > 0}<button type="button" class="inline-flex items-center gap-1 rounded-md border border-border px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted" onclick={() => (step -= 1)} disabled={busy}><ChevronLeft size={13} aria-hidden="true" /> Retour</button>{/if}
            {#if step < totalSteps - 1}
              <button type="button" class="inline-flex items-center gap-1 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover" onclick={() => (step += 1)} disabled={busy}>Suivant <ChevronRight size={13} aria-hidden="true" /></button>
            {:else}
              <button type="button" class="rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover" onclick={() => void finish()} disabled={busy}>{busy ? 'Enregistrement…' : 'Terminer'}</button>
            {/if}
          </div>
        </div>
      </footer>
    </div>
  </div>
{/if}
