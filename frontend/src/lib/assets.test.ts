import { describe, expect, it, vi } from 'vitest';
import {
  isLocalAssetPath,
  isRemoteImageSource,
  isSafeEditorImageSource,
  precomputeAssetURLs,
  scrubAbsoluteAssetURLs,
  withTimeout
} from './assets';

describe('asset path policy', () => {
  it.each([
    'assets/2026/07/photo.png',
    'assets/photo avec espaces.png'
  ])('accepts a confined asset path: %s', (path) => {
    expect(isLocalAssetPath(path)).toBe(true);
  });

  it.each([
    '../outside.png',
    'assets/../../outside.png',
    'notes/private.png',
    '/assets/photo.png',
    'https://example.com/photo.png'
  ])('rejects a non-confined asset path: %s', (path) => {
    expect(isLocalAssetPath(path)).toBe(false);
  });

  it('only allows local paths and the loopback asset server in the editor', () => {
    expect(isSafeEditorImageSource('assets/photo.png')).toBe(true);
    expect(isSafeEditorImageSource('http://127.0.0.1:43125/files/assets/photo.png')).toBe(true);
    expect(isSafeEditorImageSource('https://example.com/tracker.png')).toBe(false);
    expect(isRemoteImageSource('https://example.com/tracker.png')).toBe(true);
  });
});

describe('Markdown asset URLs', () => {
  it('resolves local assets without touching remote images', async () => {
    const resolve = vi.fn(async (path: string) => `http://127.0.0.1:1234/files/${path}`);
    const markdown = '![locale](assets/photo.png)\n![distante](https://example.com/tracker.png)';

    await expect(precomputeAssetURLs(markdown, resolve)).resolves.toBe(
      '![locale](http://127.0.0.1:1234/files/assets/photo.png)\n![distante](https://example.com/tracker.png)'
    );
    expect(resolve).toHaveBeenCalledOnce();
  });

  it('restores portable paths before saving', () => {
    expect(
      scrubAbsoluteAssetURLs('![photo](http://127.0.0.1:1234/files/assets/mes%20photos/a.png)')
    ).toBe('![photo](assets/mes photos/a.png)');
  });
});

describe('withTimeout', () => {
  it('returns a result completed before the deadline', async () => {
    await expect(withTimeout(Promise.resolve('ok'), 20, 'timeout')).resolves.toBe('ok');
  });

  it('rejects a stalled operation', async () => {
    vi.useFakeTimers();
    const result = withTimeout(new Promise<string>(() => {}), 20, 'operation expirée');
    const assertion = expect(result).rejects.toThrow('operation expirée');
    await vi.advanceTimersByTimeAsync(20);
    await assertion;
    vi.useRealTimers();
  });
});
