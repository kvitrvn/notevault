import { describe, expect, it } from 'vitest';

import { extractMermaidBlocks, normalizeMermaidCode, sha256Hex } from './mermaid-blocks';

// Fixtures partagées avec `internal/vault/pdf_test.go` : les deux côtés
// doivent produire exactement la même empreinte.
const FLOWCHART = 'flowchart TD\n  A[Début] --> B{Choix}\n  B --> C[Fin]';
const FLOWCHART_HASH = '069b1401af8eeeca1a2947183f2101712ba44d0e2e2acc9792b74e0f19c2a239';

describe('extractMermaidBlocks', () => {
  it('retourne les blocs mermaid avec leur empreinte', async () => {
    const markdown = ['# Titre', '', '```mermaid', FLOWCHART, '```', '', 'Texte.'].join('\n');
    const blocks = await extractMermaidBlocks(markdown);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].code).toBe(FLOWCHART);
    expect(blocks[0].hash).toBe(await sha256Hex(FLOWCHART));
  });

  it('ignore les blocs d’un autre langage et le code inline', async () => {
    const markdown = ['```go', 'package main', '```', '', '`mermaid`'].join('\n');
    expect(await extractMermaidBlocks(markdown)).toEqual([]);
  });

  it('accepte une info string avec métadonnées et une casse différente', async () => {
    const markdown = ['```Mermaid title="Flux"', 'graph TD; A-->B;', '```'].join('\n');
    const blocks = await extractMermaidBlocks(markdown);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].code).toBe('graph TD; A-->B;');
  });

  it('trouve les blocs imbriqués dans une liste ou une citation', async () => {
    const markdown = [
      '- item',
      '',
      '  ```mermaid',
      '  graph TD; A-->B;',
      '  ```',
      '',
      '> ```mermaid',
      '> graph LR; C-->D;',
      '> ```'
    ].join('\n');
    const blocks = await extractMermaidBlocks(markdown);
    expect(blocks.map((block) => block.code)).toEqual(['graph TD; A-->B;', 'graph LR; C-->D;']);
  });

  it('déduplique les diagrammes identiques', async () => {
    const markdown = ['```mermaid', 'graph TD; A-->B;', '```', '', '```mermaid', 'graph TD; A-->B;', '```'].join(
      '\n'
    );
    expect(await extractMermaidBlocks(markdown)).toHaveLength(1);
  });

  it('ignore un bloc mermaid vide', async () => {
    expect(await extractMermaidBlocks(['```mermaid', '', '```'].join('\n'))).toEqual([]);
  });

  it('normalise les fins de ligne CRLF avant l’empreinte', async () => {
    const crlf = ['```mermaid', 'graph TD;', '  A-->B;', '```'].join('\r\n');
    const blocks = await extractMermaidBlocks(crlf);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].code).toBe('graph TD;\n  A-->B;');
    expect(blocks[0].hash).toBe(await sha256Hex('graph TD;\n  A-->B;'));
  });
});

describe('empreinte partagée avec le backend', () => {
  it('est stable pour la fixture de référence', async () => {
    expect(await sha256Hex(FLOWCHART)).toBe(FLOWCHART_HASH);
  });

  it('normalise CR et CRLF de la même façon', () => {
    expect(normalizeMermaidCode('a\r\nb\rc')).toBe('a\nb\nc');
  });
});
