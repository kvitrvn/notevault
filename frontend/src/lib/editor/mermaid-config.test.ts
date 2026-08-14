import { describe, expect, it } from 'vitest';
import { MERMAID_BASE_CONFIG } from './mermaid';

// Régression : avec les seules clés par diagramme, Mermaid 11 rend les libellés
// en `<foreignObject><p>…<br>…</p>`. Le `<br>` HTML n'étant pas auto-fermé, le
// SVG n'est plus du XML bien formé : `sizeMermaidSVG` le rejette et l'export PDF
// retombe sur le bloc de code. Un `foreignObject` ne serait de toute façon pas
// rendu dans le `<img>` du document imprimé.
describe('MERMAID_BASE_CONFIG', () => {
  it('désactive les libellés HTML à la racine', () => {
    expect(MERMAID_BASE_CONFIG.htmlLabels).toBe(false);
  });

  it('conserve les clés par diagramme', () => {
    expect(MERMAID_BASE_CONFIG.flowchart.htmlLabels).toBe(false);
    expect(MERMAID_BASE_CONFIG.class.htmlLabels).toBe(false);
  });
});
