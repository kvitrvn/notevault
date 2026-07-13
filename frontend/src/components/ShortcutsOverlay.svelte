<script lang="ts">
  import Search from '@lucide/svelte/icons/search';
  import X from '@lucide/svelte/icons/x';
  import Keyboard from '@lucide/svelte/icons/keyboard';
  import Compass from '@lucide/svelte/icons/compass';
  import Pencil from '@lucide/svelte/icons/pencil';
  import FolderTree from '@lucide/svelte/icons/folder-tree';
  import Activity from '@lucide/svelte/icons/activity';

  type Group = {
    id: string;
    title: string;
    icon: typeof Compass;
    entries: { keys: string; label: string }[];
  };

  const groups: Group[] = [
    {
      id: 'nav',
      title: 'Navigation',
      icon: Compass,
      entries: [
        { keys: 'Ctrl+P', label: 'Recherche rapide (quick switcher)' },
        { keys: 'Ctrl+Shift+F', label: 'Focus sur la barre de filtres' },
        { keys: 'Ctrl+Shift+D', label: 'Ouvrir la note du jour' },
        { keys: 'Ctrl+Shift+H', label: 'Ouvrir l’historique de la note' },
        { keys: 'j / k', label: 'Naviguer dans la liste (haut / bas)' },
        { keys: 'h / l', label: 'Sidebar ↔ éditeur' }
      ]
    },
    {
      id: 'edit',
      title: 'Édition',
      icon: Pencil,
      entries: [
        { keys: 'Ctrl+N', label: 'Nouvelle note (avec choix du template)' },
        { keys: 'Ctrl+S', label: 'Enregistrer la note' },
        { keys: 'Ctrl+Shift+R', label: 'Renommer le titre (inline)' },
        { keys: 'Ctrl+Shift+P', label: 'Épingler / désépingler' },
        { keys: 'Ctrl+Shift+M', label: 'Déplacer la note' },
        { keys: 'Entrée', label: 'Renommer (dans le champ titre)' },
        { keys: 'Échap', label: 'Annuler le renommage' }
      ]
    },
    {
      id: 'org',
      title: 'Organisation',
      icon: FolderTree,
      entries: [
        { keys: 'Ctrl+T', label: 'Vue Tags' },
        { keys: 'Drag & drop', label: 'Déplacer une note vers un dossier' },
        { keys: 'Clic droit', label: 'Menu contextuel (renommer, dupliquer…)' }
      ]
    },
    {
      id: 'misc',
      title: 'Aide & insights',
      icon: Activity,
      entries: [
        { keys: 'Ctrl+/', label: 'Afficher cette palette' },
        { keys: 'Ctrl+Shift+G', label: 'Voir l’activité (stats locales)' }
      ]
    }
  ];

  type Props = {
    open: boolean;
    onClose: () => void;
    onReviewGuide: () => void;
  };

  let { open, onClose, onReviewGuide }: Props = $props();

  let query = $state('');
  let inputEl: HTMLInputElement | undefined = $state();

  $effect(() => {
    if (open) {
      query = '';
      requestAnimationFrame(() => inputEl?.focus());
    }
  });

  const filtered = $derived.by(() => {
    const q = query.trim().toLowerCase();
    if (!q) return groups;
    return groups
      .map((g) => ({
        ...g,
        entries: g.entries.filter(
          (e) =>
            e.label.toLowerCase().includes(q) || e.keys.toLowerCase().includes(q)
        )
      }))
      .filter((g) => g.entries.length > 0);
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

{#if open}
  <div
    class="fixed inset-0 z-[55] grid place-items-start px-4 pt-[10vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Raccourcis clavier"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative flex max-h-[78vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center gap-2 border-b border-border px-4 py-3">
        <Keyboard size={16} strokeWidth={2} class="shrink-0 text-accent" aria-hidden="true" />
        <h2 class="text-sm font-semibold text-foreground">Raccourcis clavier</h2>
        <button type="button" class="ml-auto rounded-md border border-border px-2 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground" onclick={onReviewGuide}>Revoir le guide</button>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          aria-label="Fermer"
          onclick={onClose}
        >
          <X size={13} strokeWidth={2} aria-hidden="true" />
        </button>
      </header>

      <div class="flex items-center gap-2 border-b border-border bg-background px-4 py-2">
        <Search size={14} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
        <input
          bind:this={inputEl}
          bind:value={query}
          type="search"
          class="block w-full bg-transparent py-1 text-sm text-foreground outline-none placeholder:text-faint"
          placeholder="Filtrer les raccourcis…"
          aria-label="Rechercher un raccourci"
          spellcheck="false"
          autocomplete="off"
        />
        <kbd class="shrink-0 rounded border border-border bg-panel px-1.5 py-0.5 text-xs text-subtle">esc</kbd>
      </div>

      <div class="flex-1 overflow-y-auto px-4 py-3">
        {#if filtered.length === 0}
          <p class="py-6 text-center text-sm text-subtle">
            Aucun raccourci ne correspond à « {query} ».
          </p>
        {:else}
          {#each filtered as group (group.id)}
            <section class="mb-4">
              <h3 class="mb-2 flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-subtle">
                <group.icon size={11} strokeWidth={2.5} aria-hidden="true" />
                {group.title}
              </h3>
              <ul class="flex flex-col gap-1">
                {#each group.entries as entry, i (i + entry.keys)}
                  <li class="flex items-center justify-between gap-3 rounded-md border border-border bg-background px-3 py-1.5 text-sm">
                    <span class="text-foreground">{entry.label}</span>
                    <kbd class="shrink-0 rounded border border-border-strong bg-panel px-1.5 py-0.5 font-mono text-[0.72rem] text-foreground">
                      {entry.keys}
                    </kbd>
                  </li>
                {/each}
              </ul>
            </section>
          {/each}
        {/if}
      </div>
    </div>
  </div>
{/if}
