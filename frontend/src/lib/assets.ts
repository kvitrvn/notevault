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

/** Placeholder affiché à la place d'une image dont la source sort du coffre. */
export const BLOCKED_IMAGE_SRC =
  'data:image/svg+xml;charset=utf-8,' +
  encodeURIComponent(
    '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="64">' +
      '<rect width="320" height="64" fill="none" stroke="#888" stroke-dasharray="4 4"/>' +
      '<text x="160" y="37" text-anchor="middle" font-family="sans-serif" font-size="13" fill="#888">' +
      'Image distante bloquée</text></svg>'
  );

// Le Markdown ne contient que des chemins relatifs au coffre : la résolution
// vers l'URL loopback se fait au rendu (`proxyDomURL` de Crepe), jamais dans
// le document. Rien à réécrire avant chargement ni à nettoyer avant save.

// Extrait la charge base64 d'une data URL (`data:image/png;base64,XXXX`).
// Exportée pour être testable sans FileReader.
export function base64FromDataURL(dataURL: string): string {
  const comma = dataURL.indexOf(',');
  return comma >= 0 ? dataURL.slice(comma + 1) : dataURL;
}

// Encode un fichier en base64 pour l'envoi à SaveAsset. On passe par
// FileReader plutôt que par un Uint8Array converti en tableau JS : ce dernier
// produisait un élément JSON par octet (~4x la taille binaire, et plusieurs
// secondes de thread principal bloqué sur une image de 10 Mo).
export function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result !== 'string') {
        reject(new Error('contenu de fichier illisible'));
        return;
      }
      resolve(base64FromDataURL(reader.result));
    };
    reader.onerror = () => reject(reader.error ?? new Error('lecture du fichier impossible'));
    reader.readAsDataURL(file);
  });
}

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
