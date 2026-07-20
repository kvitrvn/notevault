import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { createDebouncedTask } from './debounce';

describe('createDebouncedTask', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('runs the task once after the delay when scheduled repeatedly', () => {
    const task = vi.fn();
    const debounced = createDebouncedTask(200, task);

    debounced.schedule();
    vi.advanceTimersByTime(50);
    debounced.schedule();
    vi.advanceTimersByTime(50);
    debounced.schedule();
    vi.advanceTimersByTime(199);

    expect(task).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(task).toHaveBeenCalledTimes(1);
  });

  it('does not run the task when cancelled before the delay elapses', () => {
    const task = vi.fn();
    const debounced = createDebouncedTask(200, task);

    debounced.schedule();
    debounced.cancel();
    vi.advanceTimersByTime(500);

    expect(task).not.toHaveBeenCalled();
  });

  it('flushes a pending task immediately and skips future runs until rescheduled', () => {
    const task = vi.fn();
    const debounced = createDebouncedTask(200, task);

    debounced.schedule();
    debounced.flush();
    expect(task).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(500);
    expect(task).toHaveBeenCalledTimes(1);

    debounced.flush();
    expect(task).toHaveBeenCalledTimes(1);
  });

  it('flush is a no-op when nothing is pending', () => {
    const task = vi.fn();
    const debounced = createDebouncedTask(200, task);

    debounced.flush();
    expect(task).not.toHaveBeenCalled();
  });

  it('coalesces multiple flushes into a single run', () => {
    const task = vi.fn();
    const debounced = createDebouncedTask(200, task);

    debounced.schedule();
    debounced.flush();
    debounced.flush();

    expect(task).toHaveBeenCalledTimes(1);
  });
});