const localAssetPrefix = 'assets/';
const localAssetURL = /^http:\/\/127\.0\.0\.1:\d+\/files\/assets\//;

export function isLocalAssetPath(source: string): boolean {
  const path = source.trim().replaceAll('\\', '/');
  if (!path.startsWith(localAssetPrefix)) return false;

  const segments = path.split('/');
  return segments.length > 1 && segments.every((segment) => segment !== '' && segment !== '.' && segment !== '..');
}

export function isSafeEditorImageSource(source: unknown): boolean {
  if (typeof source !== 'string') return false;
  return isLocalAssetPath(source) || localAssetURL.test(source);
}

export function isRemoteImageSource(source: string): boolean {
  return /^https?:\/\//i.test(source.trim());
}

// Le Markdown ne contient que des chemins relatifs au coffre : la résolution
// vers l'URL loopback se fait au rendu (`proxyDomURL` de Crepe), jamais dans
// le document. Rien à réécrire avant chargement ni à nettoyer avant save.

export async function withTimeout<T>(promise: Promise<T>, ms: number, label: string): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_resolve, reject) => {
    timer = setTimeout(() => reject(new Error(label)), ms);
  });
  try {
    return await Promise.race([promise, timeout]);
  } finally {
    if (timer) clearTimeout(timer);
  }
}
