<script lang="ts">
  import X from '@lucide/svelte/icons/x';
  import Activity from '@lucide/svelte/icons/activity';
  import Tag from '@lucide/svelte/icons/tag';
  import FileText from '@lucide/svelte/icons/file-text';
  import Image from '@lucide/svelte/icons/image';
  import Type from '@lucide/svelte/icons/type';
  import { Stats } from '../../wailsjs/go/main/App';
  import type { vault } from '../../wailsjs/go/models';

  type Stats = vault.Stats;
  type DayCount = vault.DayCount;

  type Props = {
    open: boolean;
    onClose: () => void;
    onPickTag?: (tag: string) => void;
  };

  let { open, onClose, onPickTag }: Props = $props();

  let stats = $state<Stats | null>(null);
  let loading = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      void load();
    }
  });

  async function load(): Promise<void> {
    loading = true;
    error = '';
    try {
      stats = await Stats();
    } catch (err) {
      error = String(err);
    } finally {
      loading = false;
    }
  }

  function formatDay(d: string): string {
    return d.slice(5); // MM-DD
  }

  // Calcule la série alignée sur la fenêtre (30 jours).
  const series = $derived.by(() => {
    if (!stats) return { labels: [] as string[], created: [] as number[], modified: [] as number[] };
    const days = stats.createdByDay.length;
    const labelCount = stats.windowDays ?? 30;
    const createdMap = new Map<string, number>(stats.createdByDay.map((d) => [d.day, d.count]));
    const modifiedMap = new Map<string, number>(stats.modifiedByDay.map((d) => [d.day, d.count]));
    const today = new Date();
    today.setUTCHours(0, 0, 0, 0);
    const labels: string[] = [];
    const created: number[] = [];
    const modified: number[] = [];
    for (let i = labelCount - 1; i >= 0; i--) {
      const t = new Date(today);
      t.setUTCDate(today.getUTCDate() - i);
      const key = t.toISOString().slice(0, 10);
      labels.push(formatDay(key));
      created.push(createdMap.get(key) ?? 0);
      modified.push(modifiedMap.get(key) ?? 0);
    }
    void days;
    return { labels, created, modified };
  });

  const maxValue = $derived(
    Math.max(1, ...series.created, ...series.modified)
  );

  function fmtBytes(n: number): string {
    if (n < 1024) return `${n} o`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} Ko`;
    return `${(n / (1024 * 1024)).toFixed(1)} Mo`;
  }

  function fmtNumber(n: number): string {
    return new Intl.NumberFormat('fr-FR').format(n);
  }

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
    class="fixed inset-0 z-50 grid place-items-start overflow-y-auto px-4 py-8"
    role="dialog"
    aria-modal="true"
    aria-label="Activité"
  >
    <button
      class="fixed inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={onClose}
    ></button>
    <div
      class="relative mx-auto flex w-full max-w-3xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center justify-between gap-2 border-b border-border bg-background px-4 py-3">
        <h2 class="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <Activity size={15} strokeWidth={2} class="text-accent" aria-hidden="true" />
          Activité — {stats?.windowDays ?? 30} derniers jours
        </h2>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border bg-panel px-2.5 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
            onclick={() => void load()}
            disabled={loading}
          >
            {loading ? '…' : 'Rafraîchir'}
          </button>
          <button
            type="button"
            class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
            aria-label="Fermer"
            onclick={onClose}
          >
            <X size={13} strokeWidth={2} aria-hidden="true" />
          </button>
        </div>
      </header>

      <div class="flex flex-col gap-5 px-5 py-4">
        {#if loading && !stats}
          <p class="py-8 text-center text-sm text-subtle">Calcul des statistiques…</p>
        {:else if error}
          <p class="rounded-md border border-danger/40 bg-panel px-3 py-2 text-sm text-danger" role="alert">
            {error}
          </p>
        {:else if stats}
          <section class="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div class="rounded-lg border border-border bg-background p-3">
              <p class="flex items-center gap-1 text-xs text-subtle">
                <FileText size={12} strokeWidth={2} aria-hidden="true" /> Notes
              </p>
              <p class="mt-1 text-xl font-semibold text-foreground">{fmtNumber(stats.totalNotes)}</p>
            </div>
            <div class="rounded-lg border border-border bg-background p-3">
              <p class="flex items-center gap-1 text-xs text-subtle">
                <Type size={12} strokeWidth={2} aria-hidden="true" /> Mots
              </p>
              <p class="mt-1 text-xl font-semibold text-foreground">{fmtNumber(stats.totalWords)}</p>
            </div>
            <div class="rounded-lg border border-border bg-background p-3">
              <p class="flex items-center gap-1 text-xs text-subtle">
                <Image size={12} strokeWidth={2} aria-hidden="true" /> Assets
              </p>
              <p class="mt-1 text-xl font-semibold text-foreground">{fmtBytes(stats.totalAssetsBytes)}</p>
            </div>
            <div class="rounded-lg border border-border bg-background p-3">
              <p class="flex items-center gap-1 text-xs text-subtle">
                <Tag size={12} strokeWidth={2} aria-hidden="true" /> Tags distincts
              </p>
              <p class="mt-1 text-xl font-semibold text-foreground">{stats.topTags.length}</p>
            </div>
          </section>

          <section>
            <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-subtle">Modifications / jour</h3>
            <div class="flex h-32 items-end gap-0.5 rounded-lg border border-border bg-background p-3" role="img" aria-label="Histogramme des modifications par jour">
              {#each series.modified as value, i (i)}
                {@const height = maxValue > 0 ? Math.max(2, (value / maxValue) * 100) : 2}
                <div
                  class="group relative flex-1 rounded-t-sm bg-accent/40 hover:bg-accent"
                  style="height: {height}%"
                  title={value > 0 ? `${series.labels[i]} : ${value}` : series.labels[i]}
                ></div>
              {/each}
            </div>
            <div class="mt-1 flex justify-between text-[0.65rem] text-faint">
              <span>{series.labels[0]}</span>
              <span>{series.labels[Math.floor(series.labels.length / 2)]}</span>
              <span>{series.labels[series.labels.length - 1]}</span>
            </div>
          </section>

          {#if stats.topTags.length > 0}
            <section>
              <h3 class="mb-2 text-xs font-semibold uppercase tracking-wide text-subtle">Top tags</h3>
              <ul class="flex flex-wrap gap-2">
                {#each stats.topTags as tag (tag.tag)}
                  <li>
                    <button
                      type="button"
                      class="inline-flex items-center gap-1.5 rounded-md border border-border bg-background px-2.5 py-1 text-xs text-foreground hover:border-accent"
                      onclick={() => onPickTag?.(tag.tag)}
                      title="Filtrer par tag"
                    >
                      <Tag size={11} strokeWidth={2} aria-hidden="true" />
                      {tag.tag}
                      <span class="text-faint">· {tag.count}</span>
                    </button>
                  </li>
                {/each}
              </ul>
            </section>
          {/if}

          <p class="text-[0.7rem] text-faint">
            Calculé le {new Date(String(stats.computedAt)).toLocaleString('fr-FR')} —
            les jours sans activité sont représentés à zéro.
          </p>
        {/if}
      </div>
    </div>
  </div>
{/if}
