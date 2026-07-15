<script lang="ts" module>
  export type BlockValue = 'paragraph' | 'heading-1' | 'heading-2' | 'heading-3';

  /** État d'un bouton toggle : surbrillance + disponibilité contextuelle. */
  export type ToolbarItemState = {
    active: boolean;
    enabled: boolean;
  };

  /**
   * Snapshot immuable de l'état de l'éditeur consommé par la toolbar.
   * Produit par NoteEditor à chaque transaction ProseMirror ; la toolbar
   * est purement présentationnelle et n'accède jamais à l'instance Tiptap.
   * `enabled` reflète `editor.can()` : certaines commandes sont interdites
   * par le schéma selon le contexte (ex. citation dans un task item,
   * gras dans un bloc de code) — le bouton est alors grisé.
   */
  export type ToolbarState = {
    canUndo: boolean;
    canRedo: boolean;
    block: BlockValue;
    blockEnabled: boolean;
    bold: ToolbarItemState;
    italic: ToolbarItemState;
    strike: ToolbarItemState;
    bulletList: ToolbarItemState;
    orderedList: ToolbarItemState;
    taskList: ToolbarItemState;
    blockquote: ToolbarItemState;
    codeBlock: ToolbarItemState;
    canInsertTable: boolean;
    canInsertHorizontalRule: boolean;
  };
</script>

<script lang="ts">
  import Bold from '@lucide/svelte/icons/bold';
  import Code from '@lucide/svelte/icons/code';
  import Heading1 from '@lucide/svelte/icons/heading-1';
  import Heading2 from '@lucide/svelte/icons/heading-2';
  import Heading3 from '@lucide/svelte/icons/heading-3';
  import ImagePlus from '@lucide/svelte/icons/image-plus';
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

  type Props = {
    toolbar: ToolbarState;
    onUndo: () => void;
    onRedo: () => void;
    onChangeBlock: (value: BlockValue) => void;
    onToggleBold: () => void;
    onToggleItalic: () => void;
    onToggleStrike: () => void;
    onToggleBulletList: () => void;
    onToggleOrderedList: () => void;
    onToggleTaskList: () => void;
    onToggleBlockquote: () => void;
    onToggleCodeBlock: () => void;
    onInsertTable: () => void;
    onInsertImage: () => void;
    onInsertHorizontalRule: () => void;
  };

  let {
    toolbar,
    onUndo,
    onRedo,
    onChangeBlock,
    onToggleBold,
    onToggleItalic,
    onToggleStrike,
    onToggleBulletList,
    onToggleOrderedList,
    onToggleTaskList,
    onToggleBlockquote,
    onToggleCodeBlock,
    onInsertTable,
    onInsertImage,
    onInsertHorizontalRule
  }: Props = $props();

  const iconButtonBase =
    'inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-md border text-subtle transition-colors hover:text-foreground disabled:pointer-events-none disabled:opacity-40';
  const selectBase =
    'h-8 rounded-md border border-transparent bg-transparent px-2 text-sm text-foreground hover:bg-panel-muted focus:bg-panel-muted';

  function iconButtonClass(active = false): string {
    return active
      ? `${iconButtonBase} border-accent bg-accent text-accent-foreground hover:bg-accent-hover hover:text-accent-foreground`
      : `${iconButtonBase} border-transparent bg-transparent hover:bg-panel-muted`;
  }

  function handleBlockChange(event: Event): void {
    onChangeBlock((event.currentTarget as HTMLSelectElement).value as BlockValue);
  }
</script>

<div class="flex shrink-0 items-center gap-0.5 overflow-x-auto bg-background px-3 py-2" aria-label="Outils de mise en forme">
  <button class={iconButtonClass()} type="button" title="Annuler" aria-label="Annuler" onclick={onUndo} disabled={!toolbar.canUndo}>
    <Undo2 size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={`${iconButtonClass()} mr-3`} type="button" title="Rétablir" aria-label="Rétablir" onclick={onRedo} disabled={!toolbar.canRedo}>
    <Redo2 size={16} strokeWidth={2} aria-hidden="true" />
  </button>

  <label class="sr-only" for="note-editor-block-type">Style de bloc</label>
  <select
    id="note-editor-block-type"
    class={selectBase}
    value={toolbar.block}
    onchange={handleBlockChange}
    disabled={!toolbar.blockEnabled}
    title="Style de bloc"
  >
    <option value="paragraph">Texte</option>
    <option value="heading-1">Titre 1</option>
    <option value="heading-2">Titre 2</option>
    <option value="heading-3">Titre 3</option>
  </select>
  <span class="mr-3 hidden items-center px-1 text-faint sm:inline-flex" aria-hidden="true">
    {#if toolbar.block === 'heading-1'}
      <Heading1 size={16} strokeWidth={2} />
    {:else if toolbar.block === 'heading-2'}
      <Heading2 size={16} strokeWidth={2} />
    {:else if toolbar.block === 'heading-3'}
      <Heading3 size={16} strokeWidth={2} />
    {:else}
      <Pilcrow size={16} strokeWidth={2} />
    {/if}
  </span>

  <button class={iconButtonClass(toolbar.bold.active)} type="button" title="Gras" aria-label="Gras" aria-pressed={toolbar.bold.active} disabled={!toolbar.bold.enabled} onclick={onToggleBold}>
    <Bold size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={iconButtonClass(toolbar.italic.active)} type="button" title="Italique" aria-label="Italique" aria-pressed={toolbar.italic.active} disabled={!toolbar.italic.enabled} onclick={onToggleItalic}>
    <Italic size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={`${iconButtonClass(toolbar.strike.active)} mr-3`} type="button" title="Barré" aria-label="Barré" aria-pressed={toolbar.strike.active} disabled={!toolbar.strike.enabled} onclick={onToggleStrike}>
    <Strikethrough size={16} strokeWidth={2} aria-hidden="true" />
  </button>

  <button class={iconButtonClass(toolbar.bulletList.active)} type="button" title="Liste à puces" aria-label="Liste à puces" aria-pressed={toolbar.bulletList.active} disabled={!toolbar.bulletList.enabled} onclick={onToggleBulletList}>
    <List size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={iconButtonClass(toolbar.orderedList.active)} type="button" title="Liste numérotée" aria-label="Liste numérotée" aria-pressed={toolbar.orderedList.active} disabled={!toolbar.orderedList.enabled} onclick={onToggleOrderedList}>
    <ListOrdered size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={`${iconButtonClass(toolbar.taskList.active)} mr-3`} type="button" title="Liste de tâches" aria-label="Liste de tâches" aria-pressed={toolbar.taskList.active} disabled={!toolbar.taskList.enabled} onclick={onToggleTaskList}>
    <ListChecks size={16} strokeWidth={2} aria-hidden="true" />
  </button>

  <button class={iconButtonClass(toolbar.blockquote.active)} type="button" title="Citation" aria-label="Citation" aria-pressed={toolbar.blockquote.active} disabled={!toolbar.blockquote.enabled} onclick={onToggleBlockquote}>
    <Quote size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={iconButtonClass(toolbar.codeBlock.active)} type="button" title="Bloc de code" aria-label="Bloc de code" aria-pressed={toolbar.codeBlock.active} disabled={!toolbar.codeBlock.enabled} onclick={onToggleCodeBlock}>
    <Code size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={iconButtonClass()} type="button" title="Insérer un tableau" aria-label="Insérer un tableau" disabled={!toolbar.canInsertTable} onclick={onInsertTable}>
    <TableIcon size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={`${iconButtonClass()} mr-3`} type="button" title="Insérer une image" aria-label="Insérer une image" onclick={onInsertImage}>
    <ImagePlus size={16} strokeWidth={2} aria-hidden="true" />
  </button>
  <button class={iconButtonClass()} type="button" title="Insérer une séparation" aria-label="Insérer une séparation" disabled={!toolbar.canInsertHorizontalRule} onclick={onInsertHorizontalRule}>
    <Minus size={16} strokeWidth={2} aria-hidden="true" />
  </button>
</div>
