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
  /* Les sélecteurs sont préfixés par `#app` à dessein : le thème structurel de
     Crepe est importé globalement (styles.css) et cible `.milkdown .ProseMirror`,
     ce qui l'emporterait sinon sur `.chat-markdown`. Une réponse de modèle n'est
     pas une note : ni gouttière d'édition de 120 px, ni titres à 2,25 em. */
  :global(#app .chat-markdown) {
    padding: 0;
    overflow-wrap: anywhere;
    color: var(--color-foreground);
    font-size: 0.9375rem;
    line-height: 1.65;
  }

  :global(#app .chat-markdown > :first-child) {
    margin-top: 0;
  }

  :global(#app .chat-markdown > :last-child) {
    margin-bottom: 0;
  }

  :global(#app .chat-markdown p),
  :global(#app .chat-markdown ul),
  :global(#app .chat-markdown ol),
  :global(#app .chat-markdown pre),
  :global(#app .chat-markdown blockquote),
  :global(#app .chat-markdown table) {
    margin: 0.7rem 0;
  }

  :global(#app .chat-markdown h1),
  :global(#app .chat-markdown h2),
  :global(#app .chat-markdown h3),
  :global(#app .chat-markdown h4),
  :global(#app .chat-markdown h5),
  :global(#app .chat-markdown h6) {
    margin: 1.15rem 0 0.45rem;
    padding: 0;
    font-weight: 600;
    line-height: 1.35;
  }

  :global(#app .chat-markdown h1) { font-size: 1.125rem; }
  :global(#app .chat-markdown h2) { font-size: 1.0625rem; }
  :global(#app .chat-markdown h3) { font-size: 1rem; }
  :global(#app .chat-markdown h4),
  :global(#app .chat-markdown h5),
  :global(#app .chat-markdown h6) { font-size: 0.9375rem; }

  :global(#app .chat-markdown ul),
  :global(#app .chat-markdown ol) {
    padding-left: 1.35rem;
  }

  :global(#app .chat-markdown ul) { list-style: disc; }
  :global(#app .chat-markdown ol) { list-style: decimal; }

  :global(#app .chat-markdown li) {
    margin: 0.25rem 0;
    padding: 0;
  }

  :global(#app .chat-markdown li > ul),
  :global(#app .chat-markdown li > ol) {
    margin: 0.25rem 0;
  }

  :global(#app .chat-markdown li p) { margin: 0; }
  :global(#app .chat-markdown li::marker) { color: var(--color-faint); }
  :global(#app .chat-markdown strong) { font-weight: 600; }

  :global(#app .chat-markdown code) {
    border-radius: var(--radius-sm);
    background: var(--color-code);
    padding: 0.1rem 0.28rem;
    font-family: var(--font-mono);
    font-size: 0.85em;
  }

  :global(#app .chat-markdown pre) {
    overflow-x: auto;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-code);
    padding: 0.7rem 0.85rem;
    font-size: 0.8125rem;
    line-height: 1.5;
  }

  :global(#app .chat-markdown pre code) {
    padding: 0;
    background: transparent;
    font-size: inherit;
  }

  :global(#app .chat-markdown blockquote) {
    border-left: 2px solid var(--color-border-strong);
    padding-left: 0.75rem;
    color: var(--color-subtle);
  }

  :global(#app .chat-markdown table) {
    display: block;
    overflow-x: auto;
    border-collapse: collapse;
    font-size: 0.875rem;
  }

  :global(#app .chat-markdown th),
  :global(#app .chat-markdown td) {
    border: 1px solid var(--color-border);
    padding: 0.35rem 0.55rem;
    text-align: left;
    vertical-align: top;
  }

  :global(#app .chat-markdown th) {
    background: var(--color-panel-muted);
    font-weight: 600;
  }

  :global(#app .chat-markdown hr) {
    margin: 1rem 0;
    border: 0;
    border-top: 1px solid var(--color-border);
  }

  :global(#app .chat-markdown a) {
    color: var(--color-accent);
    text-decoration: underline;
    text-underline-offset: 0.15em;
    cursor: default;
  }

  :global(#app .chat-markdown img) {
    border-radius: var(--radius-md);
    max-width: 100%;
  }
</style>
