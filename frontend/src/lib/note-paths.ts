export function normalizeNotesFolderPath(path: string): string {
  const normalized = path
    .trim()
    .replace(/\\/g, '/')
    .replace(/^\/+|\/+$/g, '')
    .replace(/\/+/g, '/');

  if (!normalized || normalized === 'notes') return 'notes';
  if (normalized.startsWith('notes/')) return normalized;
  return `notes/${normalized}`;
}
