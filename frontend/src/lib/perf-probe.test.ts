import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  LONG_FRAME_MS,
  PERF_ENABLED,
  createFrameMonitor,
  percentile,
  probe,
  probeReport,
  resetProbes,
  summarizeFrameDurations
} from './perf-probe';

describe('summarizeFrameDurations', () => {
  it('returns zeroes for an empty sample', () => {
    expect(summarizeFrameDurations([])).toEqual({
      frames: 0,
      p50: 0,
      p95: 0,
      longFrames: 0,
      worstMs: 0
    });
  });

  it('reports percentiles and the worst frame', () => {
    const durations = Array.from({ length: 100 }, (_, i) => i + 1);
    const stats = summarizeFrameDurations(durations);

    expect(stats.frames).toBe(100);
    expect(stats.p50).toBe(50);
    expect(stats.p95).toBe(95);
    expect(stats.worstMs).toBe(100);
  });

  it('counts only frames strictly longer than the long-frame threshold', () => {
    const stats = summarizeFrameDurations([
      16,
      LONG_FRAME_MS,
      LONG_FRAME_MS + 0.1,
      120
    ]);

    expect(stats.longFrames).toBe(2);
  });

  it('does not mutate the input sample', () => {
    const durations = [50, 10, 30];
    summarizeFrameDurations(durations);

    expect(durations).toEqual([50, 10, 30]);
  });
});

describe('percentile', () => {
  it('returns 0 for an empty sample', () => {
    expect(percentile([], 0.95)).toBe(0);
  });

  it('clamps to the last element for fractions at or above 1', () => {
    expect(percentile([1, 2, 3], 1)).toBe(3);
    expect(percentile([1, 2, 3], 2)).toBe(3);
  });

  it('returns the first element for fractions at or below 0', () => {
    expect(percentile([1, 2, 3], 0)).toBe(1);
  });
});

describe('probe', () => {
  beforeEach(() => {
    resetProbes();
  });

  afterEach(() => {
    resetProbes();
  });

  it('returns the measured value unchanged', () => {
    expect(probe('unit', () => 42)).toBe(42);
  });

  it('propagates thrown errors', () => {
    expect(() =>
      probe('unit', () => {
        throw new Error('boom');
      })
    ).toThrow('boom');
  });

  it('accumulates call counts per label when enabled', () => {
    probe('a', () => 1);
    probe('a', () => 2);
    probe('b', () => 3);

    const rows = probeReport();
    if (!PERF_ENABLED) {
      expect(rows).toHaveLength(0);
      return;
    }
    expect(rows.find((r) => r.label === 'a')?.count).toBe(2);
    expect(rows.find((r) => r.label === 'b')?.count).toBe(1);
  });
});

describe('createFrameMonitor', () => {
  const rafCallbacks: FrameRequestCallback[] = [];

  beforeEach(() => {
    vi.useFakeTimers();
    rafCallbacks.length = 0;
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
      rafCallbacks.push(cb);
      return rafCallbacks.length;
    });
    vi.stubGlobal('cancelAnimationFrame', () => {});
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  // Hors DEV le moniteur est inerte par construction : les assertions de
  // comportement ne valent que quand les sondes sont actives.
  const itWhenEnabled = PERF_ENABLED ? it : it.skip;

  itWhenEnabled('reports once the ping burst goes idle', () => {
    const onReport = vi.fn();
    const monitor = createFrameMonitor({ idleMs: 600, onReport });

    monitor.ping();
    // La première frame est ignorée : son delta précède le début de la mesure.
    rafCallbacks.shift()?.(1000);
    rafCallbacks.shift()?.(1016);
    rafCallbacks.shift()?.(1100);

    expect(onReport).not.toHaveBeenCalled();
    vi.advanceTimersByTime(600);

    expect(onReport).toHaveBeenCalledTimes(1);
    const stats = onReport.mock.calls[0][0];
    expect(stats.frames).toBe(2);
    expect(stats.worstMs).toBe(84);
    expect(stats.longFrames).toBe(1);
  });

  itWhenEnabled('does not report when no frame was sampled', () => {
    const onReport = vi.fn();
    const monitor = createFrameMonitor({ onReport });

    monitor.ping();
    monitor.stop();

    expect(onReport).not.toHaveBeenCalled();
  });

  itWhenEnabled('starts a fresh sample after a report', () => {
    const onReport = vi.fn();
    const monitor = createFrameMonitor({ idleMs: 600, onReport });

    monitor.ping();
    rafCallbacks.shift()?.(0);
    rafCallbacks.shift()?.(16);
    vi.advanceTimersByTime(600);

    monitor.ping();
    rafCallbacks.shift()?.(5000);
    rafCallbacks.shift()?.(5020);
    vi.advanceTimersByTime(600);

    expect(onReport).toHaveBeenCalledTimes(2);
    expect(onReport.mock.calls[1][0].frames).toBe(1);
    expect(onReport.mock.calls[1][0].worstMs).toBe(20);
  });
});
