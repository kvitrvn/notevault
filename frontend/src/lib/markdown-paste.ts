type ClipboardTextSource = {
  getData(type: string): string;
};

// Returns plaintext only when the clipboard does not already provide rich
// HTML. The caller can then route it through Tiptap's Markdown parser.
export function plaintextMarkdownFromClipboard(
  clipboard: ClipboardTextSource | null | undefined
): string | null {
  if (!clipboard) return null;
  const text = clipboard.getData('text/plain').replaceAll('\r\n', '\n');
  if (text === '') return null;
  const html = clipboard.getData('text/html').trim();
  if (html !== '' && !looksLikeStructuredMarkdown(text)) return null;
  return normalizeLooseMarkdownTables(text);
}

function looksLikeStructuredMarkdown(text: string): boolean {
  return [
    /^(?: {0,3})#{1,6}\s+\S/m,
    /^(?: {0,3})(?:```|~~~)/m,
    /^\s*\|?(?:\s*:?-{3,}:?\s*\|){1,}\s*$/m,
    /^\s*[-+*]\s+\[[ xX]\]\s+\S/m,
    /^\s*(?:[-+*]|\d+[.)])\s+\S/m,
    /^\s*>\s+\S/m,
    /!\[[^\]]*]\([^)]+\)/,
    /\[[^\]]+]\([^)]+\)/,
    /(?:^|\s)(?:\*\*|__|~~)\S/
  ].some((pattern) => pattern.test(text));
}

function normalizeLooseMarkdownTables(text: string): string {
  const lines = text.split('\n');
  return lines
    .filter((line, index) => {
      if (line.trim() !== '') return true;
      const previous = lines[index - 1]?.trim() ?? '';
      const next = lines[index + 1]?.trim() ?? '';
      return !(isTableRow(previous) && isTableRow(next));
    })
    .join('\n');
}

function isTableRow(line: string): boolean {
  return line.length > 2 && line.startsWith('|') && line.endsWith('|');
}
