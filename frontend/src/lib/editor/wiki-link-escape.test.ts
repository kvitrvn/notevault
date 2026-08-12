import { describe, expect, it } from 'vitest';
import { unified } from 'unified';
import remarkParse from 'remark-parse';
import remarkStringify from 'remark-stringify';
import remarkGfm from 'remark-gfm';
import {
  MARKDOWN_STRINGIFY_OPTIONS,
  preserveWikiLinksTextHandler
} from './wiki-link-escape';

// Reproduit le câblage réel : le handler est passé dans les options de
// remark-stringify, exactement comme via `remarkStringifyOptionsCtx`.
function serialize(markdown: string, withHandler = true): string {
  const processor = unified()
    .use(remarkParse)
    .use(remarkGfm, { tablePipeAlign: false })
    .use(remarkStringify, {
      ...MARKDOWN_STRINGIFY_OPTIONS,
      ...(withHandler ? { handlers: { text: preserveWikiLinksTextHandler } } : {})
    });
  return String(processor.processSync(markdown)).trim();
}

describe('preserveWikiLinksTextHandler', () => {
  it('échappe les wiki-links sans le handler (régression à couvrir)', () => {
    expect(serialize('Voir [[Ma Note]].', false)).toBe('Voir \\[\\[Ma Note]].');
  });

  it('préserve un wiki-link simple', () => {
    expect(serialize('Voir [[Ma Note]].')).toBe('Voir [[Ma Note]].');
  });

  it.each([
    ['paragraphe', 'Voir [[Ma Note]] et [[Autre/Note]].'],
    ['liste', '- tâche [[Liée]]'],
    ['case à cocher', '- [ ] todo [[Cible]]'],
    ['citation', '> citation [[Dans Quote]]'],
    ['tableau', '| a | b |\n| - | - |\n| [[X]] | y |'],
    ['callout', '> [!IMPORTANT]\n> attention']
  ])('roundtrip dans un %s', (_label, markdown) => {
    expect(serialize(markdown)).toBe(markdown);
  });

  it('laisse le code inline intact', () => {
    expect(serialize('`code [[pas touché]]`')).toBe('`code [[pas touché]]`');
  });

  it("continue d'échapper un crochet isolé", () => {
    expect(serialize('Crochet \\[seul] ici.')).toBe('Crochet \\[seul] ici.');
  });

  it('ne casse pas un lien Markdown classique', () => {
    expect(serialize('[lien](https://exemple.test)')).toBe('[lien](https://exemple.test)');
  });

  it("n'encode pas les entités HTML (comportement Milkdown)", () => {
    expect(serialize('a < b & c [[Note]]')).toBe('a < b & c [[Note]]');
  });

  it('est idempotent', () => {
    const once = serialize('Voir [[Ma Note]] et **gras** et _italique_.');
    expect(serialize(once)).toBe(once);
  });
});
