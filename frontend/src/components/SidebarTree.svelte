<script lang="ts">
  import Folder from '@lucide/svelte/icons/folder';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import FileText from '@lucide/svelte/icons/file-text';
  import Pin from '@lucide/svelte/icons/pin';
  import Plus from '@lucide/svelte/icons/plus';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import VirtualList from './VirtualList.svelte';
  import { clickOutside } from '../lib/actions';

  type NoteSummary = {
    relativePath: string;
    title: string;
    updatedAt: string;
  };

  type Props = {
    notes: NoteSummary[];
    pinned: NoteSummary[];
    selectedPath: string;
    onOpen: (relativePath: string) => void;
    onTogglePin?: (relativePath: string) => void;
    onDragStart?: (event: DragEvent, relativePath: string) => void;
    onDragEnd?: () => void;
    onFolderDragOver?: (event: DragEvent, folder: string) => void;
    onFolderDragLeave?: (folder: string) => void;
    onFolderDrop?: (event: DragEvent, folder: string) => void;
    onContextMenu?: (event: MouseEvent, relativePath: string) => void;
    onFolderNewNote?: (folder: string) => void;
    onFolderNewFolder?: (folder: string) => void;
    dragOverFolder?: string | null;
  };

  let {
    notes,
    pinned,
    selectedPath,
    onOpen,
    onTogglePin,
    onDragStart,
    onDragEnd,
    onFolderDragOver,
    onFolderDragLeave,
    onFolderDrop,
    onContextMenu,
    onFolderNewNote,
    onFolderNewFolder,
    dragOverFolder
  }: Props = $props();

  type TreeNode = {
    name: string;
    path: string;
    children: TreeNode[];
    notes: NoteSummary[];
  };

  type FolderRow = {
    kind: 'folder';
    path: string;
    name: string;
    depth: number;
    open: boolean;
    dragOver: boolean;
    count: number;
  };

  type NoteRow = {
    kind: 'note';
    note: NoteSummary;
    depth: number;
  };

  type FlatRow = FolderRow | NoteRow;

  const ROW_HEIGHT = 30;
  const OVERSCAN = 6;

  const isDev = Boolean((import.meta as ImportMeta & { env?: { DEV?: boolean } }).env?.DEV);
  let buildSeq = 0;
  let openFolders = $state<Record<string, boolean>>({});
  let folderMenuPath = $state<string | null>(null);
  const selectedAncestors = $derived(folderAncestors(selectedPath));
  const pinnedPaths = $derived(new Set(pinned.map((p) => p.relativePath)));

  const tree = $derived.by(() => {
    const label = isDev ? `SidebarTree:build:${++buildSeq}` : '';
    if (label) console.time(label);
    const root: TreeNode = { name: '', path: '', children: [], notes: [] };
    const childMaps = new WeakMap<TreeNode, Map<string, TreeNode>>();

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
        let map = childMaps.get(cursor);
        if (!map) {
          map = new Map<string, TreeNode>();
          childMaps.set(cursor, map);
        }
        let child = map.get(cumulative);
        if (!child) {
          child = { name: parts[i], path: cumulative, children: [], notes: [] };
          map.set(cumulative, child);
          cursor.children.push(child);
        }
        cursor = child;
      }
      cursor.notes.push(note);
    }
    sortTree(root);
    if (label) console.timeEnd(label);
    return root;
  });

  const flatRows = $derived.by(() => {
    const rows: FlatRow[] = [];
    const visit = (node: TreeNode, depth: number): void => {
      for (const child of node.children) {
        const open = isOpen(child.path);
        rows.push({
          kind: 'folder',
          path: child.path,
          name: child.name,
          depth,
          open,
          dragOver: dragOverFolder === child.path,
          count: child.notes.length + child.children.length
        });
        if (open) visit(child, depth + 1);
      }
      for (const note of node.notes) {
        rows.push({ kind: 'note', note, depth });
      }
    };
    visit(tree, 0);
    return rows;
  });

  function sortTree(node: TreeNode): void {
    node.children.sort((a, b) => a.name.localeCompare(b.name));
    node.notes.sort((a, b) => a.title.localeCompare(b.title));
    node.children.forEach(sortTree);
  }

  function toggle(path: string): void {
    openFolders = { ...openFolders, [path]: !isOpen(path) };
  }

  function isOpen(path: string): boolean {
    if (selectedAncestors.has(path)) return true;
    const userValue = openFolders[path];
    if (userValue !== undefined) return userValue;
    return path === 'notes/inbox';
  }

  function isPinned(relPath: string): boolean {
    return pinnedPaths.has(relPath);
  }

  function folderAncestors(path: string): Set<string> {
    const ancestors = new Set<string>();
    const parts = path.split('/');
    let cumulative = 'notes';
    for (let i = 1; i < parts.length - 1; i++) {
      cumulative = cumulative + '/' + parts[i];
      ancestors.add(cumulative);
    }
    return ancestors;
  }
</script>

{#snippet noteRowContent(note: NoteSummary, depth: number)}
  {@const active = selectedPath === note.relativePath}
  <div
    class={active
      ? 'flex h-full w-full items-center gap-1.5 overflow-hidden rounded-md border border-accent bg-accent/15 px-2 text-foreground'
      : 'flex h-full w-full items-center gap-1.5 overflow-hidden rounded-md border border-transparent px-2 text-foreground hover:border-border hover:bg-panel-muted'}
    style="padding-left: {0.5 + depth * 0.85}rem"
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

{#snippet folderRowContent(row: FolderRow)}
  <div
    class={row.dragOver
      ? 'group relative flex h-full w-full items-center gap-1 rounded-md border border-accent bg-accent/15 px-2 text-sm font-medium text-foreground'
      : 'group relative flex h-full w-full items-center gap-1 rounded-md px-2 text-sm font-medium text-subtle hover:bg-panel-muted hover:text-foreground'}
    use:clickOutside={{ handler: () => (folderMenuPath = null), enabled: folderMenuPath === row.path }}
    style="padding-left: {0.5 + row.depth * 0.85}rem"
    ondragover={(e) => onFolderDragOver?.(e, row.path)}
    ondragleave={() => onFolderDragLeave?.(row.path)}
    ondrop={(e) => onFolderDrop?.(e, row.path)}
    role="button"
    tabindex="0"
    aria-expanded={row.open}
    onclick={() => toggle(row.path)}
    onkeydown={(e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggle(row.path);
      }
    }}
  >
    <button
      type="button"
      class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-subtle hover:bg-panel-muted hover:text-foreground"
      aria-label={row.open ? 'Replier' : 'Déplier'}
      onclick={(e) => {
        e.stopPropagation();
        toggle(row.path);
      }}
    >
      <ChevronRight
        size={11}
        strokeWidth={2.5}
        class="shrink-0 transition-transform {row.open ? 'rotate-90' : ''}"
        aria-hidden="true"
      />
    </button>
    {#if row.open}
      <FolderOpen size={13} strokeWidth={2} class="shrink-0" aria-hidden="true" />
    {:else}
      <Folder size={13} strokeWidth={2} class="shrink-0" aria-hidden="true" />
    {/if}
    <span class="min-w-0 flex-1 truncate text-left">{row.name}</span>
    <span class="shrink-0 text-xs text-faint">{row.count}</span>
    {#if onFolderNewNote || onFolderNewFolder}
      <button
        type="button"
        class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-subtle opacity-0 hover:bg-panel-muted hover:text-foreground group-hover:opacity-100 focus:opacity-100"
        title="Ajouter dans {row.name}"
        aria-label="Ajouter dans {row.name}"
        aria-haspopup="menu"
        aria-expanded={folderMenuPath === row.path}
        onclick={(e) => {
          e.stopPropagation();
          folderMenuPath = folderMenuPath === row.path ? null : row.path;
        }}
      >
        <Plus size={11} strokeWidth={2.5} aria-hidden="true" />
      </button>
    {/if}
    {#if folderMenuPath === row.path && (onFolderNewNote || onFolderNewFolder)}
      <div
        class="absolute right-1 top-full z-50 mt-1 w-44 overflow-hidden rounded-md border border-border bg-panel shadow-lg"
        role="menu"
        aria-label="Créer dans {row.name}"
        tabindex={-1}
        onclick={(e) => e.stopPropagation()}
        onkeydown={(e) => {
          if (e.key === 'Escape') folderMenuPath = null;
        }}
      >
        <button
          type="button"
          role="menuitem"
          class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-panel-muted"
          onclick={() => {
            folderMenuPath = null;
            onFolderNewNote?.(row.path);
          }}
        >
          <FileText size={13} strokeWidth={2} aria-hidden="true" />
          Nouvelle note
        </button>
        <button
          type="button"
          role="menuitem"
          class="flex w-full items-center gap-2 border-t border-border px-3 py-2 text-left text-sm hover:bg-panel-muted"
          onclick={() => {
            folderMenuPath = null;
            onFolderNewFolder?.(row.path);
          }}
        >
          <Folder size={13} strokeWidth={2} aria-hidden="true" />
          Nouveau sous-dossier
        </button>
      </div>
    {/if}
  </div>
{/snippet}

<div class="flex h-full min-h-0 flex-col px-1">
  <VirtualList
    items={flatRows}
    itemHeight={ROW_HEIGHT}
    overscan={OVERSCAN}
    class="min-h-0 flex-1"
    ariaLabel="Notes"
  >
    {#snippet children(row: FlatRow)}
      {#if row.kind === 'folder'}
        {@render folderRowContent(row)}
      {:else}
        {@render noteRowContent(row.note, row.depth)}
      {/if}
    {/snippet}
  </VirtualList>
</div>