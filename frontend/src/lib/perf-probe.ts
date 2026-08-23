// Sondes de mesure, actives uniquement en développement.
//
// Deux besoins distincts sur les longues notes :
//
//   - `probe()` accumule le coût des fonctions appelées très souvent
//     (sérialisation, comparaison « dirty », décorations wiki-link). Un
//     `console.time` par appel serait plus coûteux que le code mesuré et
//     noierait la console ; on agrège donc et on ne restitue qu'à la demande.
//   - `createFrameMonitor()` échantillonne la durée des frames pendant le
//     défilement. C'est la seule mesure qui reflète le ressenti « ça saccade » :
//     une moyenne ne dit rien, ce sont les frames longues qui se voient.
//
// Tout est inerte hors DEV : `probe` appelle directement la fonction et
// `createFrameMonitor` retourne un moniteur qui ne démarre jamais de rAF.

export const PERF_ENABLED = Boolean(
  (import.meta as ImportMeta & { env?: { DEV?: boolean } }).env?.DEV
);

/** Frame plus longue que deux périodes à 60 Hz : visible à l'œil. */
export const LONG_FRAME_MS = 32;

export type ProbeStats = {
  label: string;
  count: number;
  totalMs: number;
  maxMs: number;
  avgMs: number;
};

const probes = new Map<string, { count: number; totalMs: number; maxMs: number }>();

/**
 * Mesure `fn` et cumule son coût sous `label`. Retourne la valeur de `fn`
 * telle quelle, y compris hors DEV où aucune mesure n'est prise.
 */
export function probe<T>(label: string, fn: () => T): T {
  if (!PERF_ENABLED) return fn();
  const start = performance.now();
  try {
    return fn();
  } finally {
    const elapsed = performance.now() - start;
    const entry = probes.get(label) ?? { count: 0, totalMs: 0, maxMs: 0 };
    entry.count += 1;
    entry.totalMs += elapsed;
    if (elapsed > entry.maxMs) entry.maxMs = elapsed;
    probes.set(label, entry);
  }
}

/** Instantané des sondes, du plus coûteux au moins coûteux. */
export function probeReport(): ProbeStats[] {
  return [...probes.entries()]
    .map(([label, { count, totalMs, maxMs }]) => ({
      label,
      count,
      totalMs,
      maxMs,
      avgMs: count === 0 ? 0 : totalMs / count
    }))
    .sort((a, b) => b.totalMs - a.totalMs);
}

export function resetProbes(): void {
  probes.clear();
}

export type FrameStats = {
  frames: number;
  p50: number;
  p95: number;
  /** Nombre de frames au-delà de `LONG_FRAME_MS` — l'indicateur de saccade. */
  longFrames: number;
  worstMs: number;
};

/**
 * Percentile par rang le plus proche (pas d'interpolation) : sur quelques
 * centaines de frames, interpoler donnerait une fausse précision.
 */
export function percentile(sorted: number[], fraction: number): number {
  if (sorted.length === 0) return 0;
  const rank = Math.ceil(fraction * sorted.length);
  const index = Math.min(sorted.length - 1, Math.max(0, rank - 1));
  return sorted[index] ?? 0;
}

export function summarizeFrameDurations(durations: number[]): FrameStats {
  if (durations.length === 0) {
    return { frames: 0, p50: 0, p95: 0, longFrames: 0, worstMs: 0 };
  }
  const sorted = [...durations].sort((a, b) => a - b);
  return {
    frames: sorted.length,
    p50: percentile(sorted, 0.5),
    p95: percentile(sorted, 0.95),
    longFrames: sorted.filter((d) => d > LONG_FRAME_MS).length,
    worstMs: sorted[sorted.length - 1] ?? 0
  };
}

export type FrameMonitor = {
  /** À appeler à chaque événement de scroll : démarre ou prolonge la mesure. */
  ping: () => void;
  /** Arrête et restitue la mesure en cours, s'il y en a une. */
  stop: () => void;
};

export type FrameMonitorOptions = {
  /** Fin de salve : durée sans `ping` au bout de laquelle on restitue. */
  idleMs?: number;
  onReport: (stats: FrameStats) => void;
};

/**
 * Mesure la durée des frames tant que `ping()` est appelé, puis restitue une
 * fois la salve terminée. La première frame après le démarrage est ignorée :
 * son delta inclut le temps écoulé avant le début de la mesure.
 */
export function createFrameMonitor(options: FrameMonitorOptions): FrameMonitor {
  const idleMs = options.idleMs ?? 600;
  if (!PERF_ENABLED) return { ping: () => {}, stop: () => {} };

  let durations: number[] = [];
  let rafID: number | null = null;
  let idleTimer: ReturnType<typeof setTimeout> | null = null;
  // `null` plutôt que 0 : un timestamp rAF de 0 est légal et se confondrait
  // avec la sentinelle « pas encore de frame précédente ».
  let previous: number | null = null;

  const tick = (now: number): void => {
    if (previous !== null) durations.push(now - previous);
    previous = now;
    rafID = requestAnimationFrame(tick);
  };

  const flush = (): void => {
    if (rafID !== null) {
      cancelAnimationFrame(rafID);
      rafID = null;
    }
    if (idleTimer !== null) {
      clearTimeout(idleTimer);
      idleTimer = null;
    }
    previous = null;
    const collected = durations;
    durations = [];
    if (collected.length > 0) options.onReport(summarizeFrameDurations(collected));
  };

  return {
    ping(): void {
      if (rafID === null) {
        previous = null;
        rafID = requestAnimationFrame(tick);
      }
      if (idleTimer !== null) clearTimeout(idleTimer);
      idleTimer = setTimeout(flush, idleMs);
    },
    stop: flush
  };
}

/** Ligne unique lisible dans la console de l'inspecteur. */
export function formatFrameStats(label: string, stats: FrameStats): string {
  return (
    `[perf] ${label} — ${stats.frames} frames, ` +
    `p50 ${stats.p50.toFixed(1)} ms, p95 ${stats.p95.toFixed(1)} ms, ` +
    `pire ${stats.worstMs.toFixed(1)} ms, ` +
    `${stats.longFrames} frame(s) > ${LONG_FRAME_MS} ms`
  );
}

export function logProbeReport(label: string): void {
  if (!PERF_ENABLED) return;
  const rows = probeReport();
  if (rows.length === 0) return;
  console.log(
    `[perf] ${label} — sondes cumulées :\n` +
      rows
        .map(
          (r) =>
            `  ${r.label.padEnd(28)} ${String(r.count).padStart(6)} appels  ` +
            `total ${r.totalMs.toFixed(1)} ms  moy ${r.avgMs.toFixed(3)} ms  max ${r.maxMs.toFixed(1)} ms`
        )
        .join('\n')
  );
}

// Point d'entrée depuis la console de l'inspecteur : `make dev`, puis
//   __notevaultPerf.reset()   avant une salve de frappe ou de défilement
//   __notevaultPerf.report()  pour en lire le coût cumulé
// Sans ça les sondes ne se lisent qu'au moment où NoteEditor les émet, ce qui
// ne couvre pas une session de saisie.
if (PERF_ENABLED && typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).__notevaultPerf = {
    report: () => logProbeReport('à la demande'),
    reset: resetProbes,
    stats: probeReport
  };
}
