// Rendu des diagrammes Mermaid (flowchart, séquence, état…).
//
// Deux consommateurs :
//   - l'aperçu dans le bloc de code de l'éditeur (`renderPreview` de Crepe),
//   - le pré-rendu SVG envoyé au backend pour l'export PDF (le document
//     imprimé a une CSP `default-src 'none'` : aucun JS n'y tourne, les
//     diagrammes doivent donc arriver déjà rendus).
//
// Mermaid n'est jamais chargé au démarrage : l'import est dynamique, donc la
// bibliothèque sort du bundle principal et n'est téléchargée qu'à la première
// note contenant un diagramme. Tout est bundlé localement, aucun accès réseau.

import { LanguageDescription, LanguageSupport, StreamLanguage } from '@codemirror/language';
import type { StreamParser } from '@codemirror/language';

/** Info string reconnue sur les blocs de code fencés. */
export const MERMAID_LANGUAGE = 'mermaid';

export type MermaidTheme = 'dark' | 'light';

type MermaidAPI = (typeof import('mermaid'))['default'];

let modulePromise: Promise<MermaidAPI> | null = null;
let appliedTheme: MermaidTheme | null = null;
let queue: Promise<unknown> = Promise.resolve();
let sequence = 0;

async function loadMermaid(): Promise<MermaidAPI> {
  modulePromise ??= import('mermaid').then((module) => module.default);
  return modulePromise;
}

/**
 * Configuration commune aux deux consommateurs, thème mis à part.
 *
 * `htmlLabels: false` doit être posé **à la racine** : Mermaid 11 ignore les
 * clés par diagramme (`flowchart.htmlLabels`, `class.htmlLabels`) et continue
 * d'émettre des libellés `<foreignObject><p>…<br>…</p>`. Ces libellés sont
 * doublement inutilisables pour l'export PDF : le SVG n'est alors pas du XML
 * bien formé (le `<br>` HTML n'est pas auto-fermé, `sizeMermaidSVG` le rejette),
 * et le contenu HTML d'un `foreignObject` n'est de toute façon pas rendu quand
 * le SVG est incorporé via `<img>`. Les clés par diagramme sont conservées :
 * elles ne coûtent rien et documentent l'intention.
 */
export const MERMAID_BASE_CONFIG = {
  startOnLoad: false,
  // Assainit les libellés et neutralise les directives `click` : le contenu
  // d'une note est une entrée non fiable.
  securityLevel: 'strict',
  // Une erreur de syntaxe doit remonter par l'exception, jamais par un SVG
  // d'erreur injecté dans le document.
  suppressErrorRendering: true,
  htmlLabels: false,
  flowchart: { htmlLabels: false },
  class: { htmlLabels: false },
  // Idem : une police externe ne se charge pas dans un SVG incorporé.
  fontFamily: 'system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'
} as const;

function initialize(mermaid: MermaidAPI, theme: MermaidTheme): void {
  if (appliedTheme === theme) return;
  mermaid.initialize({
    ...MERMAID_BASE_CONFIG,
    theme: theme === 'dark' ? 'dark' : 'default'
  });
  appliedTheme = theme;
}

/**
 * Thème Mermaid correspondant au thème actif du coffre.
 *
 * Le thème est figé au moment du rendu : un aperçu déjà affiché garde ses
 * couleurs jusqu'à la prochaine édition du bloc ou la réouverture de la note.
 * Crepe ne rend l'aperçu que sur changement du texte ou du langage et
 * n'expose aucune identité de bloc, il n'y a donc pas de moyen sûr de
 * re-déclencher les aperçus en place sans risquer d'y réappliquer un contenu
 * périmé.
 */
export function currentMermaidTheme(): MermaidTheme {
  return document.documentElement.dataset.theme === 'light' ? 'light' : 'dark';
}

/**
 * Rend un diagramme en SVG. Lève si la syntaxe est invalide.
 *
 * `mermaid.initialize` est un état global : les rendus sont sérialisés pour
 * qu'un rendu clair (export PDF) ne change pas le thème d'un rendu sombre en
 * cours (aperçu de l'éditeur).
 */
export async function renderMermaid(code: string, theme: MermaidTheme): Promise<string> {
  const run = queue.then(async () => {
    const mermaid = await loadMermaid();
    initialize(mermaid, theme);
    sequence += 1;
    const { svg } = await mermaid.render(`notevault-mermaid-${sequence}`, code);
    return svg;
  });
  queue = run.catch(() => undefined);
  return run;
}

/**
 * Prépare un SVG pour l'incorporation dans le PDF, ou retourne `null` si le
 * document n'est pas exploitable.
 *
 * Deux corrections :
 *   - Mermaid émet `width="100%"` sans hauteur, ce qui ne donne aucune taille
 *     intrinsèque à un `<img src="data:image/svg+xml…">` ; les dimensions sont
 *     reprises du `viewBox`.
 *   - la sortie repasse par `XMLSerializer`, donc le backend et le moteur
 *     d'impression reçoivent du XML bien formé — un SVG servi via `<img>` qui
 *     ne l'est pas ne s'affiche pas du tout.
 */
export function sizeMermaidSVG(svg: string): string | null {
  const parsed = new DOMParser().parseFromString(svg, 'image/svg+xml');
  const root = parsed.documentElement;
  if (root.localName !== 'svg' || parsed.querySelector('parsererror')) return null;
  const box = (root.getAttribute('viewBox') ?? '')
    .trim()
    .split(/[\s,]+/)
    .map(Number);
  const [, , width, height] = box;
  if (box.length === 4 && Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0) {
    root.setAttribute('width', String(Math.round(width)));
    root.setAttribute('height', String(Math.round(height)));
    // `max-width` en ligne rétrécirait le diagramme sous sa taille utile.
    root.removeAttribute('style');
  }
  return new XMLSerializer().serializeToString(root);
}

const MERMAID_KEYWORDS =
  /^(?:graph|flowchart(?:-elk)?|subgraph|end|direction|sequenceDiagram|participant|actor|activate|deactivate|classDiagram(?:-v2)?|class|classDef|stateDiagram(?:-v2)?|state|erDiagram|journey|gantt|pie|quadrantChart|requirementDiagram|gitGraph|mindmap|timeline|sankey-beta|xychart-beta|block-beta|architecture-beta|packet-beta|title|section|note|loop|alt|else|opt|par|and|rect|critical|break|click|style|linkStyle|accTitle|accDescr)\b/;

// Coloration volontairement grossière : aucun paquet CodeMirror ne fournit de
// grammaire Mermaid, et une grammaire complète n'apporterait rien ici. Le but
// est surtout que « mermaid » existe dans le sélecteur de langage du bloc.
const mermaidParser: StreamParser<unknown> = {
  name: MERMAID_LANGUAGE,
  token(stream) {
    if (stream.match(/^%%.*/)) return 'comment';
    if (stream.match(/^"(?:[^"\\]|\\.)*"?/)) return 'string';
    if (stream.match(/^(?:-{1,3}>|={1,3}>|-\.->|<-{1,3}|-{2,3}|={2,3}|\.{2,3}|:{1,3}|\|)/)) {
      return 'operator';
    }
    if (stream.match(/^\d+(?:\.\d+)?/)) return 'number';
    if (stream.match(MERMAID_KEYWORDS)) return 'keyword';
    stream.next();
    return null;
  }
};

/**
 * Description de langage ajoutée au sélecteur du bloc de code. Le nom est
 * volontairement en minuscules : le sélecteur écrit `name` tel quel dans
 * l'info string, et le Markdown du coffre doit rester ```mermaid.
 */
export const mermaidLanguage: LanguageDescription = LanguageDescription.of({
  name: MERMAID_LANGUAGE,
  alias: ['mmd', 'flowchart'],
  load: async () => new LanguageSupport(StreamLanguage.define(mermaidParser))
});
