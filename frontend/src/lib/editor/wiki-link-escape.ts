// Sérialisation Markdown : empêche remark d'échapper `[[Titre]]`.
//
// Milkdown sérialise via remark-stringify (mdast-util-to-markdown), qui
// échappe systématiquement `[` en contenu phrasing pour qu'il ne puisse pas
// être relu comme un lien. Sans correctif, ouvrir puis sauver une note
// transforme `[[Ma Note]]` en `\[\[Ma Note]]` et casse tous les wiki-links
// du vault au premier enregistrement.
//
// Le correctif remplace le handler `text` : le texte est découpé sur les
// motifs à préserver, chaque segment ordinaire passe par l'échappement
// standard et les motifs sont recopiés tels quels. Les wiki-links restent
// donc du texte littéral dans le document ProseMirror — ce sont les
// décorations de `lib/wiki-link.ts` qui leur donnent leur apparence.
//
// Le même traitement couvre les marqueurs de callout `[!NOTE]`, `[!WARNING]`,
// etc. : même cause, même casse au save.
//
// Ce handler doit être injecté via `remarkStringifyOptionsCtx.handlers` et
// non via un plugin remark : mdast-util-to-markdown applique d'abord les
// extensions puis écrase avec `options.handlers`, où Milkdown installe déjà
// son propre handler `text`.

import type { Options } from 'remark-stringify';

/** Motifs recopiés sans échappement : `[[wiki-link]]` et `[!CALLOUT]`. */
const PRESERVED = /\[\[[^[\]\n]+\]\]|\[![A-Za-z]+\]/g;

type Handlers = NonNullable<Options['handlers']>;
type TextHandler = NonNullable<Handlers['text']>;
type SafeState = Parameters<TextHandler>[2];
type SafeInfo = Parameters<TextHandler>[3];

/**
 * Échappement d'un segment de texte ordinaire. Reproduit le handler `text`
 * de Milkdown : `encode: []` désactive l'encodage des entités, et une chaîne
 * purement blanche est renvoyée telle quelle (sinon les espaces significatifs
 * autour des marques sont mangés).
 */
function safeSegment(state: SafeState, value: string, info: SafeInfo): string {
  if (/^[^*_\\]*\s+$/.test(value)) return value;
  return state.safe(value, { ...info, encode: [] });
}

/**
 * Handler `text` de remark-stringify, à installer dans
 * `remarkStringifyOptionsCtx.handlers`.
 */
export const preserveWikiLinksTextHandler: TextHandler = (node, _parent, state, info) => {
  const value = node.value;
  PRESERVED.lastIndex = 0;
  if (!PRESERVED.test(value)) return safeSegment(state, value, info);

  PRESERVED.lastIndex = 0;
  let out = '';
  let last = 0;
  let match: RegExpExecArray | null;
  while ((match = PRESERVED.exec(value)) !== null) {
    const before = value.slice(last, match.index);
    if (before) {
      out += safeSegment(state, before, {
        ...info,
        before: out.slice(-1) || info.before,
        after: match[0].charAt(0)
      });
    }
    out += match[0];
    last = match.index + match[0].length;
  }

  const rest = value.slice(last);
  if (rest) {
    out += safeSegment(state, rest, { ...info, before: out.slice(-1) || info.before });
  }
  return out;
};

/**
 * Options de remark-stringify choisies pour coller au style Markdown déjà
 * présent dans les vaults et minimiser le diff au premier enregistrement.
 */
export const MARKDOWN_STRINGIFY_OPTIONS: Options = {
  bullet: '-',
  emphasis: '_',
  strong: '*',
  fences: true,
  rule: '-',
  listItemIndent: 'one',
  resourceLink: false
};
