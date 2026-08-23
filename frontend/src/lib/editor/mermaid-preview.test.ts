import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  DEFAULT_PREVIEW_DEBOUNCE_MS,
  createMermaidPreviewRenderer,
  type PreviewRenderer
} from './mermaid-preview';

function setup(overrides: { render?: (code: string) => Promise<string> } = {}) {
  const render = vi.fn(overrides.render ?? (async (code: string) => `<svg>${code}</svg>`));
  const renderError = vi.fn((error: unknown) => `erreur: ${String(error)}`);
  let focused: object | null = null;

  const renderPreview: PreviewRenderer = createMermaidPreviewRenderer({
    render,
    renderError,
    focusedBlock: () => focused
  });

  return {
    render,
    renderError,
    renderPreview,
    focus(block: object | null) {
      focused = block;
    }
  };
}

/** Laisse les promesses déjà résolues s'exécuter. */
const settle = () => Promise.resolve().then(() => Promise.resolve());

describe('createMermaidPreviewRenderer', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('ignores non-mermaid languages and empty content', () => {
    const { renderPreview, render } = setup();

    expect(renderPreview('typescript', 'const a = 1;', vi.fn())).toBeNull();
    expect(renderPreview('mermaid', '   \n  ', vi.fn())).toBeNull();
    expect(render).not.toHaveBeenCalled();
  });

  it('renders immediately when no block holds the focus', async () => {
    const { renderPreview, render } = setup();
    const apply = vi.fn();

    expect(renderPreview('mermaid', 'flowchart TD', apply)).toBeUndefined();
    expect(render).toHaveBeenCalledTimes(1);

    await settle();
    expect(apply).toHaveBeenCalledWith('<svg>flowchart TD</svg>');
  });

  it('accepts the language whatever its case', () => {
    const { renderPreview, render } = setup();

    renderPreview('Mermaid', 'flowchart TD', vi.fn());

    expect(render).toHaveBeenCalledTimes(1);
  });

  it('collapses a typing burst in the focused block into a single render', async () => {
    const { renderPreview, render, focus } = setup();
    const block = {};
    focus(block);
    const applies = [vi.fn(), vi.fn(), vi.fn()];

    renderPreview('mermaid', 'flowchart T', applies[0]);
    vi.advanceTimersByTime(100);
    renderPreview('mermaid', 'flowchart TD', applies[1]);
    vi.advanceTimersByTime(100);
    renderPreview('mermaid', 'flowchart TD\n  A-->B', applies[2]);

    expect(render).not.toHaveBeenCalled();
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);

    expect(render).toHaveBeenCalledTimes(1);
    expect(render).toHaveBeenCalledWith('flowchart TD\n  A-->B');

    await settle();
    expect(applies[0]).not.toHaveBeenCalled();
    expect(applies[1]).not.toHaveBeenCalled();
    expect(applies[2]).toHaveBeenCalledWith('<svg>flowchart TD\n  A-->B</svg>');
  });

  // Le pire symptôme serait un aperçu resté sur « Rendu du diagramme… » :
  // une demande visant un AUTRE bloc ne doit jamais être abandonnée.
  it('never drops a pending request belonging to another block', async () => {
    const { renderPreview, render, focus } = setup();
    const typed = {};
    const other = {};
    const typedApply = vi.fn();
    const otherApply = vi.fn();

    focus(typed);
    renderPreview('mermaid', 'flowchart TD', typedApply);
    expect(render).not.toHaveBeenCalled();

    focus(other);
    renderPreview('mermaid', 'sequenceDiagram', otherApply);

    // La demande du premier bloc part sans attendre au lieu d'être écrasée.
    expect(render).toHaveBeenCalledTimes(1);
    expect(render).toHaveBeenCalledWith('flowchart TD');

    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();

    expect(render).toHaveBeenCalledTimes(2);
    expect(typedApply).toHaveBeenCalledWith('<svg>flowchart TD</svg>');
    expect(otherApply).toHaveBeenCalledWith('<svg>sequenceDiagram</svg>');
  });

  it('flushes a pending request when an unfocused render arrives', async () => {
    const { renderPreview, render, focus } = setup();
    const typedApply = vi.fn();
    const mountApply = vi.fn();

    focus({});
    renderPreview('mermaid', 'flowchart TD', typedApply);
    focus(null);
    renderPreview('mermaid', 'stateDiagram-v2', mountApply);

    expect(render).toHaveBeenCalledTimes(2);
    await settle();
    expect(typedApply).toHaveBeenCalledWith('<svg>flowchart TD</svg>');
    expect(mountApply).toHaveBeenCalledWith('<svg>stateDiagram-v2</svg>');
  });

  it('replays the last rendered value without calling mermaid again', async () => {
    const { renderPreview, render, focus } = setup();
    const block = {};
    focus(block);

    renderPreview('mermaid', 'flowchart TD', vi.fn());
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();
    expect(render).toHaveBeenCalledTimes(1);

    expect(renderPreview('mermaid', 'flowchart TD', vi.fn())).toBe('<svg>flowchart TD</svg>');
    expect(render).toHaveBeenCalledTimes(1);
  });

  it('drops a pending request made obsolete by a replayed value', async () => {
    const { renderPreview, render, focus } = setup();
    const block = {};
    focus(block);

    renderPreview('mermaid', 'flowchart TD', vi.fn());
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();

    // Saisie puis retour arrière : on revient à un état déjà rendu.
    renderPreview('mermaid', 'flowchart TDX', vi.fn());
    expect(renderPreview('mermaid', 'flowchart TD', vi.fn())).toBe('<svg>flowchart TD</svg>');

    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS * 4);
    expect(render).toHaveBeenCalledTimes(1);
  });

  it('reports an invalid diagram instead of leaving the panel loading', async () => {
    const failure = new Error('Parse error');
    const { renderPreview, renderError, focus } = setup({
      render: async () => {
        throw failure;
      }
    });
    const apply = vi.fn();
    focus({});

    renderPreview('mermaid', 'flowchart ??', apply);
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();

    expect(renderError).toHaveBeenCalledWith(failure);
    expect(apply).toHaveBeenCalledWith(`erreur: ${String(failure)}`);
  });

  it('does not cache a failed render', async () => {
    let attempts = 0;
    const { renderPreview, render, focus } = setup({
      render: async (code) => {
        attempts += 1;
        if (attempts === 1) throw new Error('Parse error');
        return `<svg>${code}</svg>`;
      }
    });
    const block = {};
    focus(block);

    renderPreview('mermaid', 'flowchart TD', vi.fn());
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();

    const apply = vi.fn();
    expect(renderPreview('mermaid', 'flowchart TD', apply)).toBeUndefined();
    vi.advanceTimersByTime(DEFAULT_PREVIEW_DEBOUNCE_MS);
    await settle();

    expect(render).toHaveBeenCalledTimes(2);
    expect(apply).toHaveBeenCalledWith('<svg>flowchart TD</svg>');
  });
});
