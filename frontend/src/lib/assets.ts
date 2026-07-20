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

// Convertit uniquement les assets du coffre. Les URL distantes restent dans
// le Markdown, mais l'éditeur les rend sous forme de contenu bloqué.
export async function precomputeAssetURLs(
  markdown: string,
  resolve: (relativePath: string) => Promise<string>
): Promise<string> {
  const imagePattern = /!\[([^\]]*)\]\(([^)]+)\)/g;
  const replacements = new Map<string, string>();

  for (const match of markdown.matchAll(imagePattern)) {
    const source = match[2].trim();
    if (!isLocalAssetPath(source)) continue;
    const absolute = await resolve(source);
    if (absolute !== source) {
      replacements.set(match[0], `![${match[1]}](${absolute})`);
    }
  }

  let output = markdown;
  for (const [original, replacement] of replacements) {
    output = output.replaceAll(original, replacement);
  }
  return output;
}

// Les URL loopback ne doivent jamais être persistées dans les fichiers .md.
// On retire aussi la query string (`?t=...` ajouté par le serveur d'assets
// pour son token de session) pour ne garder que le chemin relatif propre.
export function scrubAbsoluteAssetURLs(markdown: string): string {
  return markdown.replace(
    /(!\[[^\]]*\]\()http:\/\/127\.0\.0\.1:\d+\/files\/(assets\/[^?)]+)(\?[^)]*)?(\))/g,
    (_match, prefix: string, relativePath: string, _query: string | undefined, suffix: string) =>
      `${prefix}${decodeURI(relativePath)}${suffix}`
  );
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
