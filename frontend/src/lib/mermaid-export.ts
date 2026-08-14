// Pré-rendu des diagrammes d'une note pour l'export PDF.
//
// Le document imprimé interdit tout script (CSP `default-src 'none'`) : les
// diagrammes doivent être rendus ici, puis transmis au backend indexés par
// l'empreinte de leur code source. Un diagramme manquant n'est pas une erreur —
// le bloc retombe simplement sur son rendu en code.

import { renderMermaid, sizeMermaidSVG } from './editor/mermaid';
import { extractMermaidBlocks } from './mermaid-blocks';

/** Aligné sur `maxMermaidDiagrams` (internal/vault/pdf_mermaid.go). */
export const MAX_EXPORT_DIAGRAMS = 40;

/**
 * Rend en SVG les blocs ```mermaid de `markdown`. Toujours résolu : un échec
 * de rendu retire le diagramme du lot sans interrompre l'export.
 */
export async function renderMermaidDiagramsForExport(
  markdown: string
): Promise<Record<string, string>> {
  const diagrams: Record<string, string> = {};
  if (markdown.trim() === '') return diagrams;

  let blocks;
  try {
    blocks = await extractMermaidBlocks(markdown);
  } catch {
    console.error('[export] mermaid: analyse du Markdown impossible');
    return diagrams;
  }
  // Au-delà du plafond le backend rejette tout le lot : inutile de rendre.
  if (blocks.length === 0 || blocks.length > MAX_EXPORT_DIAGRAMS) return diagrams;

  let failures = 0;
  for (const block of blocks) {
    try {
      // Thème clair systématiquement : le PDF est imprimé sur fond blanc,
      // indépendamment du thème du coffre.
      const svg = sizeMermaidSVG(await renderMermaid(block.code, 'light'));
      if (svg) diagrams[block.hash] = svg;
      else failures += 1;
    } catch {
      // Ni le code du diagramme ni le message de Mermaid ne sont journalisés :
      // ils contiennent du contenu de note.
      failures += 1;
    }
  }
  if (failures > 0) {
    console.error(`[export] mermaid: ${failures}/${blocks.length} diagramme(s) non rendu(s)`);
  }
  return diagrams;
}
