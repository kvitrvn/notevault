<script lang="ts">
  import FilePlus2 from '@lucide/svelte/icons/file-plus-2';
  import X from '@lucide/svelte/icons/x';
  import Sparkles from '@lucide/svelte/icons/sparkles';

  type Template = { id: string; name: string; body: string; builtin: boolean };

  type Props = {
    open: boolean;
    templates: Template[];
    onPick: (templateId: string, title: string) => void;
    onClose: () => void;
  };

  let { open, templates, onPick, onClose }: Props = $props();

  let title = $state('');
  let selected = $state('blank');

  $effect(() => {
    if (open) {
      title = '';
      selected = 'blank';
      requestAnimationFrame(() => titleEl?.focus());
    }
  });

  let titleEl: HTMLInputElement | undefined = $state();

  function commit(): void {
    const t = title.trim() || 'Nouvelle note';
    onPick(selected, t);
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

  function previewBody(t: Template): string {
    return t.body.split('\n').slice(0, 4).join('\n');
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 pt-[8vh]"
    role="dialog"
    aria-modal="true"
    aria-label="Nouvelle note"
  >
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative flex max-h-[85vh] w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <div class="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
        <h2 class="flex items-center gap-1.5 text-base font-semibold text-foreground">
          <FilePlus2 size={16} strokeWidth={2} aria-hidden="true" />
          Nouvelle note
        </h2>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          aria-label="Fermer"
          title="Fermer (Esc)"
          onclick={onClose}
        >
          <X size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>

      <div class="flex flex-col gap-3 px-4 py-3">
        <label class="flex flex-col gap-1">
          <span class="text-xs font-medium text-subtle">Titre</span>
          <input
            bind:this={titleEl}
            type="text"
            bind:value={title}
            placeholder="Titre de la note"
            class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
            spellcheck="false"
            autocomplete="off"
            onkeydown={(e) => {
              if (e.key === 'Enter' && !e.metaKey && !e.ctrlKey) {
                e.preventDefault();
                commit();
              }
            }}
          />
        </label>

        <div class="flex flex-col gap-1">
          <span class="text-xs font-medium text-subtle">Modèle</span>
          <ul
            class="grid max-h-[55vh] grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2"
            role="listbox"
            aria-label="Modèles"
          >
            {#each templates as tpl (tpl.id)}
              {@const active = selected === tpl.id}
              <li>
                <button
                  type="button"
                  class={active
                    ? 'flex w-full flex-col gap-1 rounded-md border border-accent bg-accent/10 p-3 text-left shadow-sm'
                    : 'flex w-full flex-col gap-1 rounded-md border border-border bg-background p-3 text-left hover:border-accent/60 hover:bg-panel-muted'}
                  role="option"
                  aria-selected={active}
                  onclick={() => (selected = tpl.id)}
                >
                  <span class="flex items-center gap-1.5 text-sm font-medium text-foreground">
                    {#if tpl.builtin}
                      <Sparkles size={12} strokeWidth={2} class="text-accent" aria-hidden="true" />
                    {/if}
                    {tpl.name}
                  </span>
                  <pre
                    class="overflow-hidden whitespace-pre-wrap break-words rounded bg-background/60 p-2 text-xs text-subtle"
                  >{previewBody(tpl) || '(vide)'}</pre>
                </button>
              </li>
            {/each}
          </ul>
        </div>
      </div>

      <div class="flex items-center justify-between gap-3 border-t border-border bg-background px-4 py-2.5 text-xs text-subtle">
        <span>
          <kbd class="rounded border border-border-strong bg-panel px-1.5 py-0.5">Ctrl+Enter</kbd>
          pour créer
        </span>
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
            <FilePlus2 size={13} strokeWidth={2} aria-hidden="true" />
            Créer la note
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}