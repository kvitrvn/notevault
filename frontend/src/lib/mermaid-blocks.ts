// Repérage des blocs ```mermaid d'une note, en vue du pré-rendu pour l'export
// PDF.
//
// Le HTML produit pour l'impression a une CSP `default-src 'none'` : aucun JS
// n'y tourne, Mermaid ne peut donc pas s'exécuter côté Go. Le frontend rend
// les diagrammes puis les transmet, indexés par une empreinte du code source.
// Le backend recalcule la même empreinte sur l'AST goldmark et incorpore le
// SVG correspondant ; une empreinte absente retombe sur le bloc de code.
//
// Contrat d'empreinte partagé avec `internal/vault/pdf.go` :
//   sha256 hexadécimal minuscule du corps du bloc, fins de ligne normalisées
//   en `\n`, sans saut de ligne final, sans autre normalisation d'espaces.

import remarkGfm from 'remark-gfm';
import remarkParse from 'remark-parse';
import { unified } from 'unified';

import { MERMAID_LANGUAGE } from './editor/mermaid';

export type MermaidBlock = {
  /** Corps du bloc, fins de ligne normalisées. */
  code: string;
  /** sha256 hexadécimal de `code`. */
  hash: string;
};

type MarkdownNode = {
  type: string;
  lang?: string | null;
  value?: string;
  children?: MarkdownNode[];
};

/** Normalisation des fins de ligne, appliquée des deux côtés de l'empreinte. */
export function normalizeMermaidCode(code: string): string {
  return code.replace(/\r\n?/g, '\n');
}

export async function sha256Hex(value: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(value));
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('');
}

function collectCodeNodes(node: MarkdownNode, into: MarkdownNode[]): void {
  if (node.type === 'code') into.push(node);
  for (const child of node.children ?? []) collectCodeNodes(child, into);
}

/**
 * Blocs de code fencés dont l'info string est `mermaid`, dédupliqués par
 * empreinte : deux diagrammes identiques ne sont rendus qu'une fois.
 */
export async function extractMermaidBlocks(markdown: string): Promise<MermaidBlock[]> {
  const tree = unified().use(remarkParse).use(remarkGfm).parse(markdown) as MarkdownNode;
  const codeNodes: MarkdownNode[] = [];
  collectCodeNodes(tree, codeNodes);

  const blocks: MermaidBlock[] = [];
  const seen = new Set<string>();
  for (const node of codeNodes) {
    if ((node.lang ?? '').toLowerCase() !== MERMAID_LANGUAGE) continue;
    const code = normalizeMermaidCode(node.value ?? '');
    if (code.trim() === '') continue;
    const hash = await sha256Hex(code);
    if (seen.has(hash)) continue;
    seen.add(hash);
    blocks.push({ code, hash });
  }
  return blocks;
}
