<script lang="ts">
  import Filter from '@lucide/svelte/icons/filter';
  import X from '@lucide/svelte/icons/x';

  type Chip = { kind: string; text: string };

  type Props = {
    value: string;
    chips: Chip[];
    onChange: (value: string) => void;
    onRemoveChip: (kind: string, text: string) => void;
    onClear: () => void;
  };

  let { value, chips, onChange, onRemoveChip, onClear }: Props = $props();

  let inputEl: HTMLInputElement | undefined = $state();

  export function focus(): void {
    inputEl?.focus();
    inputEl?.select();
  }
</script>

<div class="flex flex-col gap-2">
  <div
    class="flex items-center gap-2 rounded-md border border-border bg-background px-2 py-1.5 focus-within:border-accent"
  >
    <Filter size={14} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
    <input
      bind:this={inputEl}
      type="search"
      value={value}
      oninput={(e) => onChange((e.currentTarget as HTMLInputElement).value)}
      placeholder="Filtrer… ex. tag:projet updated:today"
      class="block w-full bg-transparent text-sm text-foreground outline-none placeholder:text-faint"
      aria-label="Filtre de la sidebar"
      spellcheck="false"
      autocomplete="off"
    />
    {#if value || chips.length > 0}
      <button
        type="button"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-subtle hover:bg-panel-muted hover:text-foreground"
        title="Effacer les filtres"
        aria-label="Effacer les filtres"
        onclick={onClear}
      >
        <X size={12} strokeWidth={2} aria-hidden="true" />
      </button>
    {/if}
  </div>

  {#if chips.length > 0}
    <div class="flex flex-wrap items-center gap-1.5 px-1">
      {#each chips as chip (chip.kind + ':' + chip.text)}
        <button
          type="button"
          class="inline-flex items-center gap-1 rounded-full border border-border bg-panel-muted px-2 py-0.5 text-xs text-foreground hover:border-danger/45 hover:text-danger"
          title="Retirer ce filtre"
          onclick={() => onRemoveChip(chip.kind, chip.text)}
        >
          <span>{chip.text}</span>
          <X size={10} strokeWidth={2.5} aria-hidden="true" />
        </button>
      {/each}
    </div>
  {/if}
</div>