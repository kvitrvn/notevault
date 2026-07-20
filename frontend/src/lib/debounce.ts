export interface DebouncedTask {
  schedule: () => void;
  flush: () => void;
  cancel: () => void;
}

export function createDebouncedTask(delayMs: number, task: () => void): DebouncedTask {
  let timer: ReturnType<typeof setTimeout> | null = null;
  const run = (): void => {
    timer = null;
    task();
  };
  return {
    schedule(): void {
      if (timer) clearTimeout(timer);
      timer = setTimeout(run, delayMs);
    },
    flush(): void {
      if (timer === null) return;
      clearTimeout(timer);
      run();
    },
    cancel(): void {
      if (timer === null) return;
      clearTimeout(timer);
      timer = null;
    }
  };
}