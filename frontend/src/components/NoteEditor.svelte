<script lang="ts">
  import Bold from '@lucide/svelte/icons/bold';
  import Code from '@lucide/svelte/icons/code';
  import Heading1 from '@lucide/svelte/icons/heading-1';
  import Heading2 from '@lucide/svelte/icons/heading-2';
  import Heading3 from '@lucide/svelte/icons/heading-3';
  import Italic from '@lucide/svelte/icons/italic';
  import List from '@lucide/svelte/icons/list';
  import ListChecks from '@lucide/svelte/icons/list-checks';
  import ListOrdered from '@lucide/svelte/icons/list-ordered';
  import Minus from '@lucide/svelte/icons/minus';
  import Pilcrow from '@lucide/svelte/icons/pilcrow';
  import Quote from '@lucide/svelte/icons/quote';
  import Redo2 from '@lucide/svelte/icons/redo-2';
  import Strikethrough from '@lucide/svelte/icons/strikethrough';
  import TableIcon from '@lucide/svelte/icons/table';
  import Undo2 from '@lucide/svelte/icons/undo-2';
  import { onDestroy, onMount } from 'svelte';
  import { Editor } from '@tiptap/core';
  import StarterKit from '@tiptap/starter-kit';
  import { Markdown } from '@tiptap/markdown';
  import { TableKit } from '@tiptap/extension-table';
  import { TaskItem, TaskList } from '@tiptap/extension-list';

  export let markdown = '';
  export let onChange: (value: string) => void = () => {};

  type BlockValue = 'paragraph' | 'heading-1' | 'heading-2' | 'heading-3';

  let host: HTMLDivElement;
  let editor: Editor | null = null;
  let lastMarkdown = markdown;
  let editorVersion = 0;

  const iconButtonBase =
    'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border text-subtle transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40';
  const selectBase =
    'h-8 rounded-md border border-transparent bg-transparent px-2 text-sm text-foreground hover:bg-panel-muted focus:bg-panel-muted';

  function bumpEditorState(): void {
    editorVersion += 1;
  }

  function iconButtonClass(active = false): string {
    return active
      ? `${iconButtonBase} border-accent bg-accent text-accent-foreground hover:bg-accent-hover hover:text-accent-foreground`
      : `${iconButtonBase} border-transparent bg-transparent hover:bg-panel-muted`;
  }

  function canUndo(): boolean {
    editorVersion;
    return editor?.can().undo() ?? false;
  }

  function canRedo(): boolean {
    editorVersion;
    return editor?.can().redo() ?? false;
  }

  function isActive(name: string, attributes: Record<string, unknown> | undefined = undefined): boolean {
    editorVersion;
    return editor?.isActive(name, attributes) ?? false;
  }

  function currentBlock(): BlockValue {
    editorVersion;

    if (editor?.isActive('heading', { level: 1 })) return 'heading-1';
    if (editor?.isActive('heading', { level: 2 })) return 'heading-2';
    if (editor?.isActive('heading', { level: 3 })) return 'heading-3';

    return 'paragraph';
  }

  onMount(() => {
    editor = new Editor({
      element: host,
      extensions: [
        StarterKit,
        TableKit,
        TaskList,
        TaskItem.configure({
          nested: true
        }),
        Markdown
      ],
      content: markdown,
      contentType: 'markdown',
      editorProps: {
        attributes: {
          class: 'note-editor-content',
          spellcheck: 'true'
        }
      },
      onCreate: bumpEditorState,
      onSelectionUpdate: bumpEditorState,
      onTransaction: bumpEditorState,
      onUpdate: ({ editor: updatedEditor }) => {
        const value = updatedEditor.getMarkdown();
        lastMarkdown = value;
        onChange(value);
        bumpEditorState();
      }
    });
  });

  // Ne recharge l'éditeur que lorsqu'une autre note est ouverte ou que le
  // contenu est remplacé depuis App.svelte. `emitUpdate: false` empêche une
  // boucle de mises à jour entre le parent et Tiptap.
  $: if (editor && markdown !== lastMarkdown) {
    editor.commands.setContent(markdown, {
      contentType: 'markdown',
      emitUpdate: false
    });
    lastMarkdown = markdown;
    bumpEditorState();
  }

  function undo(): void {
    editor?.chain().focus().undo().run();
  }

  function redo(): void {
    editor?.chain().focus().redo().run();
  }

  function toggleBold(): void {
    editor?.chain().focus().toggleBold().run();
  }

  function toggleItalic(): void {
    editor?.chain().focus().toggleItalic().run();
  }

  function toggleStrike(): void {
    editor?.chain().focus().toggleStrike().run();
  }

  function changeBlock(event: Event): void {
    const value = (event.currentTarget as HTMLSelectElement).value as BlockValue;
    const chain = editor?.chain().focus();

    if (!chain) return;

    if (value === 'paragraph') {
      chain.setParagraph().run();
      return;
    }

    const level = Number(value.replace('heading-', '')) as 1 | 2 | 3;
    chain.toggleHeading({ level }).run();
  }

  function toggleBulletList(): void {
    editor?.chain().focus().toggleBulletList().run();
  }

  function toggleOrderedList(): void {
    editor?.chain().focus().toggleOrderedList().run();
  }

  function toggleTaskList(): void {
    editor?.chain().focus().toggleTaskList().run();
  }

  function toggleBlockquote(): void {
    editor?.chain().focus().toggleBlockquote().run();
  }

  function toggleCodeBlock(): void {
    editor?.chain().focus().toggleCodeBlock().run();
  }

  function insertTable(): void {
    editor
      ?.chain()
      .focus()
      .insertTable({
        rows: 3,
        cols: 3,
        withHeaderRow: true
      })
      .run();
  }

  function insertHorizontalRule(): void {
    editor?.chain().focus().setHorizontalRule().run();
  }

  onDestroy(() => {
    editor?.destroy();
  });
</script>

<div class="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-background text-foreground">
  <div class="flex shrink-0 items-center gap-0.5 overflow-x-auto bg-background px-3 py-2" aria-label="Outils de mise en forme">
    <button class={iconButtonClass()} type="button" title="Annuler" aria-label="Annuler" onclick={undo} disabled={!canUndo()}>
      <Undo2 size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass()} mr-3`} type="button" title="Rétablir" aria-label="Rétablir" onclick={redo} disabled={!canRedo()}>
      <Redo2 size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <label class="sr-only" for="note-editor-block-type">Style de bloc</label>
    <select id="note-editor-block-type" class={selectBase} value={currentBlock()} onchange={changeBlock} title="Style de bloc">
      <option value="paragraph">Texte</option>
      <option value="heading-1">Titre 1</option>
      <option value="heading-2">Titre 2</option>
      <option value="heading-3">Titre 3</option>
    </select>
    <span class="mr-3 hidden items-center px-1 text-faint sm:inline-flex" aria-hidden="true">
      {#if currentBlock() === 'heading-1'}
        <Heading1 size={16} strokeWidth={2} />
      {:else if currentBlock() === 'heading-2'}
        <Heading2 size={16} strokeWidth={2} />
      {:else if currentBlock() === 'heading-3'}
        <Heading3 size={16} strokeWidth={2} />
      {:else}
        <Pilcrow size={16} strokeWidth={2} />
      {/if}
    </span>

    <button class={iconButtonClass(isActive('bold'))} type="button" title="Gras" aria-label="Gras" aria-pressed={isActive('bold')} onclick={toggleBold}>
      <Bold size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('italic'))} type="button" title="Italique" aria-label="Italique" aria-pressed={isActive('italic')} onclick={toggleItalic}>
      <Italic size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass(isActive('strike'))} mr-3`} type="button" title="Barré" aria-label="Barré" aria-pressed={isActive('strike')} onclick={toggleStrike}>
      <Strikethrough size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <button class={iconButtonClass(isActive('bulletList'))} type="button" title="Liste à puces" aria-label="Liste à puces" aria-pressed={isActive('bulletList')} onclick={toggleBulletList}>
      <List size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('orderedList'))} type="button" title="Liste numérotée" aria-label="Liste numérotée" aria-pressed={isActive('orderedList')} onclick={toggleOrderedList}>
      <ListOrdered size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={`${iconButtonClass(isActive('taskList'))} mr-3`} type="button" title="Liste de tâches" aria-label="Liste de tâches" aria-pressed={isActive('taskList')} onclick={toggleTaskList}>
      <ListChecks size={16} strokeWidth={2} aria-hidden="true" />
    </button>

    <button class={iconButtonClass(isActive('blockquote'))} type="button" title="Citation" aria-label="Citation" aria-pressed={isActive('blockquote')} onclick={toggleBlockquote}>
      <Quote size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass(isActive('codeBlock'))} type="button" title="Bloc de code" aria-label="Bloc de code" aria-pressed={isActive('codeBlock')} onclick={toggleCodeBlock}>
      <Code size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass()} type="button" title="Insérer un tableau" aria-label="Insérer un tableau" onclick={insertTable}>
      <TableIcon size={16} strokeWidth={2} aria-hidden="true" />
    </button>
    <button class={iconButtonClass()} type="button" title="Insérer une séparation" aria-label="Insérer une séparation" onclick={insertHorizontalRule}>
      <Minus size={16} strokeWidth={2} aria-hidden="true" />
    </button>
  </div>

  <div class="min-h-0 flex-1 overflow-auto bg-background text-foreground" bind:this={host}></div>
</div>

<style>
  :global(.note-editor-content) {
    min-height: 100%;
    width: 100%;
    padding: 1.25rem 1rem 4rem;
    outline: none;
    background: var(--color-background);
    color: var(--color-foreground);
    line-height: 1.7;
    caret-color: var(--color-foreground);
  }

  :global(.note-editor-content:focus) {
    outline: none;
  }

  :global(.note-editor-content > :first-child) {
    margin-top: 0;
  }

  :global(.note-editor-content h1),
  :global(.note-editor-content h2),
  :global(.note-editor-content h3) {
    margin-top: 1.6rem;
    margin-bottom: 0.7rem;
    color: var(--color-foreground);
    line-height: 1.15;
  }

  :global(.note-editor-content h1) {
    font-size: 2.15rem;
    font-weight: 720;
    letter-spacing: 0;
  }

  :global(.note-editor-content h2) {
    font-size: 1.55rem;
    font-weight: 680;
    letter-spacing: 0;
  }

  :global(.note-editor-content h3) {
    font-size: 1.2rem;
    font-weight: 650;
    letter-spacing: 0;
  }

  :global(.note-editor-content p) {
    margin: 0.75rem 0;
  }

  :global(.note-editor-content strong) {
    font-weight: 700;
  }

  :global(.note-editor-content hr) {
    margin: 1.75rem 0;
    border: 0;
    border-top: 1px solid var(--color-border);
  }

  :global(.note-editor-content ul),
  :global(.note-editor-content ol) {
    margin: 0.75rem 0;
    padding-left: 1.55rem;
  }

  :global(.note-editor-content li) {
    margin: 0.25rem 0;
  }

  :global(.note-editor-content li::marker) {
    color: var(--color-subtle);
  }

  :global(.note-editor-content ul[data-type='taskList']) {
    padding-left: 0;
    list-style: none;
  }

  :global(.note-editor-content li[data-type='taskItem']) {
    display: flex;
    gap: 0.6rem;
    align-items: flex-start;
  }

  :global(.note-editor-content li[data-type='taskItem'] > label) {
    flex: 0 0 auto;
    margin-top: 0.18rem;
  }

  :global(.note-editor-content input[type='checkbox']) {
    width: 1rem;
    height: 1rem;
    accent-color: var(--color-accent);
  }

  :global(.note-editor-content table) {
    width: 100%;
    margin: 1rem 0;
    border-collapse: collapse;
    overflow: hidden;
    border-radius: var(--radius-md);
  }

  :global(.note-editor-content th),
  :global(.note-editor-content td) {
    min-width: 7rem;
    padding: 0.6rem 0.7rem;
    border: 1px solid var(--color-border);
    color: var(--color-foreground);
    vertical-align: top;
  }

  :global(.note-editor-content th) {
    background: var(--color-panel-muted);
    font-weight: 650;
  }

  :global(.note-editor-content pre) {
    overflow-x: auto;
    margin: 1rem 0;
    padding: 0.95rem 1rem;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-lg);
    background: var(--color-code);
    color: var(--color-foreground);
  }

  :global(.note-editor-content code) {
    padding: 0.08rem 0.24rem;
    border-radius: var(--radius-sm);
    background: var(--color-code);
    color: var(--color-foreground);
    font-size: 0.92em;
  }

  :global(.note-editor-content pre code) {
    padding: 0;
    background: transparent;
    font-size: 0.9rem;
  }

  :global(.note-editor-content blockquote) {
    margin: 1rem 0;
    padding: 0.1rem 0 0.1rem 1rem;
    border-left: 3px solid var(--color-accent);
    color: var(--color-subtle);
  }

  :global(.note-editor-content a) {
    color: var(--color-accent);
    text-decoration: underline;
    text-underline-offset: 0.15em;
  }

  :global(.note-editor-content .ProseMirror-selectednode) {
    outline: 2px solid var(--color-focus);
  }

  @media (max-width: 640px) {
    :global(.note-editor-content) {
      padding: 1rem 0.75rem 3rem;
    }
  }
</style>
