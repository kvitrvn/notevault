<script lang="ts">
  import { onMount } from 'svelte';
  import Check from '@lucide/svelte/icons/check';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import Sparkles from '@lucide/svelte/icons/sparkles';
  import Sun from '@lucide/svelte/icons/sun';
  import Moon from '@lucide/svelte/icons/moon';
  import Keyboard from '@lucide/svelte/icons/keyboard';
  import { MarkOnboardingCompleted } from '../../wailsjs/go/main/App';
  import { vault } from '../../wailsjs/go/models';
  const { Onboarding } = vault;

  type ThemeChoice = 'dark' | 'light';
  type OnboardingType = InstanceType<typeof Onboarding>;

  type Props = {
    open: boolean;
    initialTheme: ThemeChoice;
    onDone: (skipped: boolean) => void;
  };

  let { open, initialTheme, onDone }: Props = $props();

  let step = $state(0);
  let theme = $state<ThemeChoice>('dark');
  let busy = $state(false);

  const totalSteps = 3;
  const labels = ['Bienvenue', 'Apparence', 'Raccourcis'];

  const shortcuts: { keys: string; label: string }[] = [
    { keys: 'Ctrl+P', label: 'Recherche rapide' },
    { keys: 'Ctrl+N', label: 'Nouvelle note' },
    { keys: 'Ctrl+S', label: 'Enregistrer' },
    { keys: 'Ctrl+T', label: 'Vue Tags' },
    { keys: 'Ctrl+Shift+P', label: 'Épingler la note' },
    { keys: 'Ctrl+Shift+D', label: 'Note du jour' },
    { keys: 'Ctrl+Shift+M', label: 'Déplacer' },
    { keys: 'Ctrl+Shift+R', label: 'Renommer' },
    { keys: 'Ctrl+Shift+H', label: 'Historique' },
    { keys: 'Ctrl+Shift+F', label: 'Filtres sidebar' },
    { keys: 'Ctrl+/', label: 'Cette aide' },
    { keys: 'j / k', label: 'Naviguer dans la liste' }
  ];

  onMount(() => {
    if (open) {
      step = 0;
      theme = initialTheme;
    }
  });

  $effect(() => {
    if (open) {
      document.documentElement.dataset.theme = theme;
    }
  });

  async function finish(skipped: boolean): Promise<void> {
    busy = true;
    try {
      const onboarding = new Onboarding({
        theme,
        skipped,
        completedAt: new Date()
      }) as OnboardingType;
      await MarkOnboardingCompleted(onboarding);
      // Persist theme via existing toggleTheme logic from the parent.
      window.localStorage.setItem('notevault-theme', theme);
      onDone(skipped);
    } catch (err) {
      // En cas d'échec de persistance, on ferme quand même pour ne pas
      // bloquer l'utilisateur. La configuration du thème reste appliquée
      // pour la session.
      console.error('[onboarding] mark failed:', err);
      onDone(skipped);
    } finally {
      busy = false;
    }
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      void finish(true);
    } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      if (step < totalSteps - 1) step += 1;
      else void finish(false);
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-[60] grid place-items-center px-4"
    role="dialog"
    aria-modal="true"
    aria-label="Onboarding"
  >
    <div class="absolute inset-0 bg-black/65" aria-hidden="true"></div>
    <div
      class="relative flex w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center justify-between border-b border-border bg-background px-5 py-3">
        <div class="flex items-center gap-2 text-sm font-semibold text-foreground">
          <Sparkles size={15} strokeWidth={2} class="text-accent" aria-hidden="true" />
          {labels[step]}
        </div>
        <div class="flex items-center gap-1.5 text-xs text-subtle">
          {#each { length: totalSteps } as _, i (i)}
            <span
              class={i === step
                ? 'h-1.5 w-4 rounded-full bg-accent'
                : 'h-1.5 w-1.5 rounded-full bg-border-strong'}
              aria-hidden="true"
            ></span>
          {/each}
          <span class="ml-1 text-xs text-faint">{step + 1}/{totalSteps}</span>
        </div>
      </header>

      <div class="min-h-[18rem] px-6 py-6">
        {#if step === 0}
          <h1 class="text-2xl font-semibold text-foreground">Bienvenue dans NoteVault</h1>
          <p class="mt-2 text-sm leading-6 text-subtle">
            NoteVault est un bloc-notes 100% local. Vos notes sont stockées
            dans un coffre de fichiers Markdown sur votre machine — vous
            pouvez les ouvrir dans n'importe quel autre éditeur.
          </p>
          <ul class="mt-4 flex flex-col gap-2 text-sm text-foreground">
            <li class="flex items-start gap-2">
              <Check size={14} strokeWidth={2.5} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" />
              Éditeur Tiptap avec wiki-links, images et coloration syntaxique.
            </li>
            <li class="flex items-start gap-2">
              <Check size={14} strokeWidth={2.5} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" />
              Recherche full-text, filtres par tag/dossier, épingles.
            </li>
            <li class="flex items-start gap-2">
              <Check size={14} strokeWidth={2.5} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" />
              Historique local, export, thèmes personnalisés.
            </li>
          </ul>
        {:else if step === 1}
          <h1 class="text-2xl font-semibold text-foreground">Choisissez votre apparence</h1>
          <p class="mt-2 text-sm leading-6 text-subtle">
            Vous pourrez changer à tout moment depuis l'en-tête. Les thèmes
            personnalisés se placent dans
            <code class="rounded bg-background px-1.5 py-0.5">.notevault/themes/</code>.
          </p>
          <div class="mt-5 grid grid-cols-2 gap-3">
            <button
              type="button"
              class={theme === 'dark'
                ? 'flex flex-col items-center gap-2 rounded-xl border-2 border-accent bg-panel-muted p-4 text-sm text-foreground'
                : 'flex flex-col items-center gap-2 rounded-xl border border-border bg-panel p-4 text-sm text-foreground hover:border-border-strong'}
              aria-pressed={theme === 'dark'}
              onclick={() => (theme = 'dark')}
            >
              <div class="grid h-16 w-16 place-items-center rounded-lg border border-border-strong bg-[#151515] text-[#ecebe7]">
                <Moon size={22} strokeWidth={2} aria-hidden="true" />
              </div>
              Sombre
            </button>
            <button
              type="button"
              class={theme === 'light'
                ? 'flex flex-col items-center gap-2 rounded-xl border-2 border-accent bg-panel-muted p-4 text-sm text-foreground'
                : 'flex flex-col items-center gap-2 rounded-xl border border-border bg-panel p-4 text-sm text-foreground hover:border-border-strong'}
              aria-pressed={theme === 'light'}
              onclick={() => (theme = 'light')}
            >
              <div class="grid h-16 w-16 place-items-center rounded-lg border border-border-strong bg-[#f6f5f2] text-[#202124]">
                <Sun size={22} strokeWidth={2} aria-hidden="true" />
              </div>
              Clair
            </button>
          </div>
        {:else}
          <h1 class="flex items-center gap-2 text-2xl font-semibold text-foreground">
            <Keyboard size={20} strokeWidth={2} class="text-accent" aria-hidden="true" />
            Raccourcis essentiels
          </h1>
          <p class="mt-2 text-sm leading-6 text-subtle">
            Toute la liste est accessible plus tard avec
            <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+/</kbd>.
          </p>
          <ul class="mt-4 grid grid-cols-1 gap-1.5 sm:grid-cols-2">
            {#each shortcuts as s (s.keys)}
              <li class="flex items-center justify-between gap-2 rounded-md border border-border bg-background px-2.5 py-1.5 text-xs">
                <span class="text-subtle">{s.label}</span>
                <kbd class="shrink-0 rounded border border-border-strong bg-panel px-1.5 py-0.5 font-mono text-[0.7rem] text-foreground">
                  {s.keys}
                </kbd>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      <footer class="flex items-center justify-between gap-2 border-t border-border bg-background px-5 py-3 text-xs text-subtle">
        <button
          type="button"
          class="rounded-md px-2 py-1 text-subtle hover:text-foreground"
          onclick={() => void finish(true)}
          disabled={busy}
        >
          Passer
        </button>
        <div class="flex items-center gap-2">
          {#if step > 0}
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
              onclick={() => (step -= 1)}
              disabled={busy}
            >
              <ChevronLeft size={13} strokeWidth={2} aria-hidden="true" />
              Retour
            </button>
          {/if}
          {#if step < totalSteps - 1}
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
              onclick={() => (step += 1)}
              disabled={busy}
            >
              Suivant
              <ChevronRight size={13} strokeWidth={2} aria-hidden="true" />
            </button>
          {:else}
            <button
              type="button"
              class="inline-flex items-center gap-1 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
              onclick={() => void finish(false)}
              disabled={busy}
            >
              Commencer
              <Check size={13} strokeWidth={2} aria-hidden="true" />
            </button>
          {/if}
        </div>
      </footer>
    </div>
  </div>
{/if}
