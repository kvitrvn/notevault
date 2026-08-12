import { describe, expect, it, vi } from 'vitest';
import {
  base64FromDataURL,
  isLocalAssetPath,
  isRemoteImageSource,
  isSafeEditorImageSource,
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

describe('base64FromDataURL', () => {
  it('strips the data URL prefix', () => {
    expect(base64FromDataURL('data:image/png;base64,iVBORw0KGgo=')).toBe('iVBORw0KGgo=');
  });

  it('keeps a payload that contains no comma', () => {
    expect(base64FromDataURL('iVBORw0KGgo=')).toBe('iVBORw0KGgo=');
  });

  it('splits on the first comma only', () => {
    // Le base64 standard n'utilise jamais la virgule, mais on ne veut pas
    // dépendre de cette propriété pour découper.
    expect(base64FromDataURL('data:text/plain;base64,YQ==,YQ==')).toBe('YQ==,YQ==');
  });

  it('produces what Go base64.StdEncoding decodes', () => {
    // Vecteur de contrôle : "NoteVault" en base64 standard, tel que
    // base64.StdEncoding.DecodeString le relit côté SaveAsset.
    const encoded = btoa('NoteVault');
    expect(encoded).toBe('Tm90ZVZhdWx0');
    expect(base64FromDataURL(`data:application/octet-stream;base64,${encoded}`)).toBe('Tm90ZVZhdWx0');
  });
});
