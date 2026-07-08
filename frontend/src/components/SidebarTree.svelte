<script lang="ts">
  import Folder from '@lucide/svelte/icons/folder';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import FileText from '@lucide/svelte/icons/file-text';
  import Pin from '@lucide/svelte/icons/pin';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import GripVertical from '@lucide/svelte/icons/grip-vertical';

  type NoteSummary = {
    relativePath: string;
    title: string;
    updatedAt: string;
  };

  type FolderInfo = { path: string; name: string; count: number };

  type Props = {
    notes: NoteSummary[];
    pinned: NoteSummary[];
    folders: FolderInfo[];
    selectedPath: string;
    onOpen: (relativePath: string) => void;
    onTogglePin?: (relativePath: string) => void;
    onDragStart?: (event: DragEvent, relativePath: string) => void;
    onDragEnd?: () => void;
    onFolderDragOver?: (event: DragEvent, folder: string) => void;
    onFolderDragLeave?: (folder: string) => void;
    onFolderDrop?: (event: DragEvent, folder: string) => void;
    onContextMenu?: (event: MouseEvent, relativePath: string) => void;
    dragOverFolder?: string | null;
  };

  let {
    notes,
    pinned,
    folders,
    selectedPath,
    onOpen,
    onTogglePin,
    onDragStart,
    onDragEnd,
    onFolderDragOver,
    onFolderDragLeave,
    onFolderDrop,
    onContextMenu,
    dragOverFolder
  }: Props = $props();

  type TreeNode = {
    name: string;
    path: string;
    children: TreeNode[];
    notes: NoteSummary[];
  };

  let openFolders = $state<Record<string, boolean>>({ inbox: true });

  const tree = $derived.by(() => {
    const root: TreeNode = { name: '', path: '', children: [], notes: [] };
    for (const note of notes) {
      const parts = note.relativePath.split('/');
      if (parts.length < 3) {
        root.notes.push(note);
        continue;
      }
      let cursor = root;
      let cumulative = 'notes';
      for (let i = 1; i < parts.length - 1; i++) {
        cumulative = cumulative + '/' + parts[i];
        let child = cursor.children.find((c) => c.path === cumulative);
        if (!child) {
          child = { name: parts[i], path: cumulative, children: [], notes: [] };
          cursor.children.push(child);
        }
        cursor = child;
      }
      cursor.notes.push(note);
    }
    sortTree(root);
    return root;
  });

  function sortTree(node: TreeNode): void {
    node.children.sort((a, b) => a.name.localeCompare(b.name));
    node.notes.sort((a, b) => a.title.localeCompare(b.title));
    node.children.forEach(sortTree);
  }

  function toggle(path: string): void {
    openFolders = { ...openFolders, [path]: !openFolders[path] };
  }

  function isOpen(path: string): boolean {
    return openFolders[path] !== false;
  }

  function isPinned(relPath: string): boolean {
    return pinned.some((p) => p.relativePath === relPath);
  }

  function isDragOver(path: string): boolean {
    return dragOverFolder === path;
  }
</script>

{#snippet noteRow(note: NoteSummary, indent: number)}
  {@const active = selectedPath === note.relativePath}
  <div
    class={active
      ? 'group flex w-full items-center gap-1.5 rounded-md border border-accent bg-accent/15 px-2 py-1 text-foreground'
      : 'group flex w-full items-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-foreground hover:border-border hover:bg-panel-muted'}
    style="padding-left: {0.5 + indent * 0.85}rem"
    role="button"
    tabindex="0"
    aria-current={active ? 'page' : undefined}
    draggable="true"
    ondragstart={(e) => onDragStart?.(e, note.relativePath)}
    ondragend={() => onDragEnd?.()}
    onclick={() => onOpen(note.relativePath)}
    oncontextmenu={(e) => onContextMenu?.(e, note.relativePath)}
    onkeydown={(e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        onOpen(note.relativePath);
      }
    }}
  >
    <GripVertical size={10} strokeWidth={2} class="shrink-0 text-faint" aria-hidden="true" />
    {#if isPinned(note.relativePath)}
      <Pin size={11} strokeWidth={2.5} class="shrink-0 text-accent" aria-label="épinglée" />
    {:else}
      <FileText size={11} strokeWidth={2} class="shrink-0 text-subtle" aria-hidden="true" />
    {/if}
    <span class="min-w-0 flex-1 truncate text-sm">{note.title || 'Sans titre'}</span>
    {#if onTogglePin}
      <span
        role="button"
        tabindex="0"
        class={active
          ? 'inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-accent hover:bg-panel-muted'
          : 'hidden h-5 w-5 shrink-0 items-center justify-center rounded text-subtle group-hover:inline-flex hover:bg-panel-muted hover:text-foreground'}
        title="Désépingler (Ctrl+Shift+P)"
        aria-label="Désépingler"
        onclick={(e) => {
          e.stopPropagation();
          onTogglePin?.(note.relativePath);
        }}
        onkeydown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.stopPropagation();
            e.preventDefault();
            onTogglePin?.(note.relativePath);
          }
        }}
      >
        <Pin size={11} strokeWidth={2.5} aria-hidden="true" />
      </span>
    {/if}
  </div>
{/snippet}

{#snippet folderNode(node: TreeNode, depth: number)}
  {@const open = isOpen(node.path)}
  {@const dragOver = isDragOver(node.path)}
  <div>
    <button
      type="button"
      class={dragOver
        ? 'flex w-full items-center gap-1 rounded-md border border-accent bg-accent/15 px-2 py-1 text-left text-sm font-medium text-foreground'
        : 'flex w-full items-center gap-1 rounded-md px-2 py-1 text-left text-sm font-medium text-subtle hover:bg-panel-muted hover:text-foreground'}
      style="padding-left: {0.5 + depth * 0.85}rem"
      onclick={() => toggle(node.path)}
      aria-expanded={open}
      ondragover={(e) => onFolderDragOver?.(e, node.path)}
      ondragleave={() => onFolderDragLeave?.(node.path)}
      ondrop={(e) => onFolderDrop?.(e, node.path)}
    >
      <ChevronRight
        size={11}
        strokeWidth={2.5}
        class="shrink-0 transition-transform {open ? 'rotate-90' : ''}"
        aria-hidden="true"
      />
      {#if open}
        <FolderOpen size={13} strokeWidth={2} class="shrink-0" aria-hidden="true" />
      {:else}
        <Folder size={13} strokeWidth={2} class="shrink-0" aria-hidden="true" />
      {/if}
      <span class="min-w-0 flex-1 truncate">{node.name}</span>
      <span class="text-xs text-faint">{node.notes.length + node.children.length}</span>
    </button>
    {#if open}
      {#each node.children as child (child.path)}
        {@render folderNode(child, depth + 1)}
      {/each}
      {#each node.notes as note (note.relativePath)}
        {@render noteRow(note, depth + 1)}
      {/each}
    {/if}
  </div>
{/snippet}

<div class="flex flex-col gap-0.5 px-1">
  {#each tree.children as child (child.path)}
    {@render folderNode(child, 0)}
  {/each}
  {#each tree.notes as note (note.relativePath)}
    {@render noteRow(note, 0)}
  {/each}
</div>