<script lang="ts">
  import { onMount } from 'svelte';
  import { defaultValueCtx, Editor, editorViewOptionsCtx, rootCtx } from '@milkdown/kit/core';
  import { commonmark } from '@milkdown/kit/preset/commonmark';
  import { gfm } from '@milkdown/kit/preset/gfm';
  import { BLOCKED_IMAGE_SRC, isSafeEditorImageSource } from '../lib/assets';

  type Props = {
    markdown: string;
  };

  let { markdown }: Props = $props();
  let host: HTMLDivElement | undefined;

  // Rendu seul : pas de Crepe ici (ni slash menu, ni poignée de bloc, ni
  // CodeMirror) — juste le parseur Markdown de Milkdown en lecture seule.
  // Les liens ne sont jamais suivis : la webview ne doit pas naviguer.
  onMount(() => {
    if (!host) return;
    let editor: Editor | null = null;

    void Editor.make()
      .config((ctx) => {
        ctx.set(rootCtx, host as HTMLElement);
        ctx.set(defaultValueCtx, markdown);
        ctx.update(editorViewOptionsCtx, (prev) => ({
          ...prev,
          editable: () => false,
          attributes: { class: 'chat-markdown', 'aria-readonly': 'true' },
          // Une réponse de modèle peut contenir `![](https://…)`. Sans ce
          // nodeView, le schéma image de commonmark recopie `node.attrs` tel
          // quel dans le <img> et la webview part chercher l'image — pixel
          // espion dans une application qui ne doit rien émettre. Crepe fait
          // l'équivalent côté éditeur via `proxyDomURL`.
          nodeViews: {
            image: (node: { attrs: Record<string, unknown> }) => {
              const dom = document.createElement('img');
              const src = typeof node.attrs.src === 'string' ? node.attrs.src : '';
              dom.setAttribute('src', isSafeEditorImageSource(src) ? src : BLOCKED_IMAGE_SRC);
              if (typeof node.attrs.alt === 'string') dom.setAttribute('alt', node.attrs.alt);
              if (typeof node.attrs.title === 'string') dom.setAttribute('title', node.attrs.title);
              return { dom };
            }
          },
          handleDOMEvents: {
            click: (_view, event) => {
              const target = event.target;
              if (!(target instanceof Element) || !target.closest('a')) return false;
              event.preventDefault();
              return true;
            }
          }
        }));
      })
      .use(commonmark)
      .use(gfm)
      .create()
      .then((instance) => {
        editor = instance;
      })
      .catch((err) => console.error('[chat-markdown] init failed:', err));

    return () => {
      void editor?.destroy();
    };
  });
</script>

<div bind:this={host}></div>

<style>
  :global(.chat-markdown) {
    overflow-wrap: anywhere;
    color: var(--color-foreground);
    font-size: 0.875rem;
    line-height: 1.55rem;
  }

  :global(.chat-markdown > :first-child) {
    margin-top: 0;
  }

  :global(.chat-markdown > :last-child) {
    margin-bottom: 0;
  }

  :global(.chat-markdown p),
  :global(.chat-markdown ul),
  :global(.chat-markdown ol),
  :global(.chat-markdown pre),
  :global(.chat-markdown blockquote) {
    margin: 0.55rem 0;
  }

  :global(.chat-markdown h1),
  :global(.chat-markdown h2),
  :global(.chat-markdown h3) {
    margin: 1rem 0 0.4rem;
    font-weight: 650;
    line-height: 1.3;
  }

  :global(.chat-markdown h1) { font-size: 1.1rem; }
  :global(.chat-markdown h2) { font-size: 1rem; }
  :global(.chat-markdown h3) { font-size: 0.925rem; }

  :global(.chat-markdown ul),
  :global(.chat-markdown ol) {
    padding-left: 1.25rem;
  }

  :global(.chat-markdown ul) { list-style: disc; }
  :global(.chat-markdown ol) { list-style: decimal; }
  :global(.chat-markdown li) { margin: 0.2rem 0; }
  :global(.chat-markdown li::marker) { color: var(--color-subtle); }
  :global(.chat-markdown strong) { font-weight: 700; }

  :global(.chat-markdown code) {
    border-radius: var(--radius-sm);
    background: var(--color-code);
    padding: 0.08rem 0.24rem;
    font-size: 0.9em;
  }

  :global(.chat-markdown pre) {
    overflow-x: auto;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-code);
    padding: 0.65rem 0.75rem;
  }

  :global(.chat-markdown pre code) {
    padding: 0;
    background: transparent;
  }

  :global(.chat-markdown blockquote) {
    border-left: 2px solid var(--color-border-strong);
    padding-left: 0.75rem;
    color: var(--color-subtle);
  }

  :global(.chat-markdown hr) {
    margin: 0.9rem 0;
    border: 0;
    border-top: 1px solid var(--color-border);
  }

  :global(.chat-markdown a) {
    color: var(--color-accent);
    text-decoration: underline;
    text-underline-offset: 0.15em;
    cursor: default;
  }
</style>
