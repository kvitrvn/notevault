export type TaggedNote = {
  relativePath: string;
  tags?: string[];
};

export function resolveChatPaths(
  notes: TaggedNote[],
  manualPaths: string[],
  selectedTags: string[],
  excludedPaths: string[]
): string[] {
  const manual = new Set(manualPaths);
  const excluded = new Set(excludedPaths);
  return notes
    .filter((note) => {
      const selectedByTag = selectedTags.some((tag) => (note.tags ?? []).includes(tag));
      return (manual.has(note.relativePath) || selectedByTag) && !excluded.has(note.relativePath);
    })
    .map((note) => note.relativePath);
}
