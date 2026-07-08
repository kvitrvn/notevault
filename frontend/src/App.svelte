<script lang="ts">
  import Moon from '@lucide/svelte/icons/moon';
  import Plus from '@lucide/svelte/icons/plus';
  import Save from '@lucide/svelte/icons/save';
  import Sun from '@lucide/svelte/icons/sun';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Search from '@lucide/svelte/icons/search';
  import Pin from '@lucide/svelte/icons/pin';
  import PinOff from '@lucide/svelte/icons/pin-off';
  import CalendarDays from '@lucide/svelte/icons/calendar-days';
  import LayoutList from '@lucide/svelte/icons/layout-list';
  import FolderTree from '@lucide/svelte/icons/folder-tree';

  import NoteEditor from './components/NoteEditor.svelte';
  import SaveIndicator from './components/SaveIndicator.svelte';
  import VirtualList from './components/VirtualList.svelte';
  import QuickSwitcher from './components/QuickSwitcher.svelte';
  import FilterBar from './components/FilterBar.svelte';
  import SidebarTree from './components/SidebarTree.svelte';
  import type { SaveState } from './components/SaveIndicator.svelte';
  import { domain, vault } from '../wailsjs/go/models';

  import {
    CreateNote,
    DeleteNote,
    EnsureDailyNote,
    GetConfig,
    IsNotePinned,
    ListFolders,
    ListNotes,
    ListNotesFiltered,
    ListPinned,
    ListTags,
    OpenDailyNote,
    OpenNote,
    PinNote,
    SaveNote,
    VaultPath
  } from '../wailsjs/go/main/App';

  type Note = domain.Note;
  type NoteSummary = domain.NoteSummary;
  type FolderInfo = vault.FolderInfo;
  type FilterQuery = vault.FilterQuery;

  const AUTO_SAVE_DEBOUNCE_MS = 1500;

  let notes: NoteSummary[] = $state([]);
  let pinned: NoteSummary[] = $state([]);
  let folders: FolderInfo[] = $state([]);
  let selected: Note | null = $state<Note | null>(null);
  let lastSavedSnapshot = '';
  let vaultPath = $state('');
  let loading = $state(true);
  let saving = $state(false);
  let deleting = $state(false);
  let confirmingDelete = $state(false);
  let error = $state('');
  let saveState: SaveState = $state('clean');
  let lastSavedAt: Date | null = $state(null);
  let toast: { kind: 'info' | 'error'; message: string; id: number } | null = $state(null);
  let toastSeq = 0;
  let autoSaveTimer: ReturnType<typeof setTimeout> | null = null;

  let theme: 'dark' | 'light' = $state(
    document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'
  );

  // Vue sidebar
  let view: 'flat' | 'tree' = $state(
    (window.localStorage.getItem('notevault-view') as 'flat' | 'tree') || 'flat'
  );
  let sidebarFocused = $state(false);
  let activeFilter = $state('');
  let activeChips: { kind: string; text: string }[] = $state([]);
  let parsedFilter: {
    Query: string;
    Tags: string[];
    ExcludeTags: string[];
    Path: string;
    UpdatedFrom?: string;
    UpdatedTo?: string;
  } | null = $state(null);

  // Quick switcher
  let quickSwitcherOpen = $state(false);
  let allEntries: { relativePath: string; title: string; updatedAt: string; score: number }[] = $state([]);

  // Pin state pour la note courante
  let isCurrentPinned = $state(false);

  let filterBar = $state<FilterBar>();
  let sidebarEl: HTMLElement | undefined = $state();

  const currentTitle = $derived(selected?.title?.trim() || 'Aucune note');
  const selectedPath = $derived(selected?.relativePath || '');
  const hasUnsavedChanges = $derived(
    selected !== null && snapshot(selected) !== lastSavedSnapshot
  );
  const pinnedSet = $derived(new Set(pinned.map((p) => p.relativePath)));

  function snapshot(note: Note): string {
    return JSON.stringify({
      title: note.title,
      content: note.content,
      tags: note.tags ?? []
    });
  }

  function formatDate(value: unknown): string {
    const date = new Date(String(value ?? ''));
    return Number.isNaN(date.getTime())
      ? ''
      : date.toLocaleDateString('fr-FR', {
          day: '2-digit',
          month: '2-digit',
          year: 'numeric'
        });
  }

  function showToast(kind: 'info' | 'error', message: string): void {
    toastSeq += 1;
    const id = toastSeq;
    toast = { kind, message, id };
    setTimeout(() => {
      if (toast?.id === id) toast = null;
    }, 4000);
  }

  function setView(v: 'flat' | 'tree'): void {
    view = v;
    window.localStorage.setItem('notevault-view', v);
  }

  function buildQuery(): FilterQuery {
    return vault.FilterQuery.createFrom({
      Query: parsedFilter?.Query ?? '',
      Tags: parsedFilter?.Tags ?? [],
      ExcludeTags: parsedFilter?.ExcludeTags ?? [],
      Path: parsedFilter?.Path ?? '',
      UpdatedFrom: parsedFilter?.UpdatedFrom,
      UpdatedTo: parsedFilter?.UpdatedTo
    });
  }

  async function refresh(): Promise<void> {
    loading = true;
    error = '';

    try {
      const cfg = await GetConfig();
      const autoDaily = cfg?.autoDailyNote === true;
      const fq = buildQuery();
      const fetchAll = !activeFilter.trim() && activeChips.length === 0;
      const [list, pin, fold, vp] = await Promise.all([
        fetchAll ? ListNotes() : ListNotesFiltered(fq, 1000),
        ListPinned(),
        ListFolders(),
        VaultPath()
      ]);
      notes = list;
      pinned = pin;
      folders = fold;
      vaultPath = vp;
      // Reconstruit les entrées pour le quick switcher (toutes les notes).
      if (fetchAll) {
        allEntries = list.map((n) => ({
          relativePath: n.relativePath,
          title: n.title,
          updatedAt: String(n.updatedAt ?? ''),
          score: 0
        }));
      } else {
        // Re-fetch all pour le quick switcher (cheap).
        const all = await ListNotes();
        allEntries = all.map((n) => ({
          relativePath: n.relativePath,
          title: n.title,
          updatedAt: String(n.updatedAt ?? ''),
          score: 0
        }));
      }
      // Auto-daily : on s'assure que la note du jour existe (sans ouvrir).
      if (autoDaily) {
        try {
          await EnsureDailyNote();
        } catch {
          /* non bloquant */
        }
      }
    } catch (err) {
      error = String(err);
    } finally {
      loading = false;
    }
  }

  async function refreshPinnedAndFolders(): Promise<void> {
    try {
      const [pin, fold] = await Promise.all([ListPinned(), ListFolders()]);
      pinned = pin;
      folders = fold;
    } catch {
      /* silencieux */
    }
  }

  async function openNote(relativePath: string): Promise<void> {
    if (!(await flushSave())) return;
    error = '';

    try {
      const note = await OpenNote(relativePath);
      selected = note;
      lastSavedSnapshot = snapshot(note);
      saveState = 'clean';
      isCurrentPinned = await IsNotePinned(relativePath);
    } catch (err) {
      error = String(err);
    }
  }

  async function createNote(): Promise<void> {
    if (!(await flushSave())) return;
    error = '';

    try {
      const note = await CreateNote('Nouvelle note', '');
      selected = note;
      lastSavedSnapshot = snapshot(note);
      saveState = 'clean';
      lastSavedAt = new Date();
      await refresh();
      isCurrentPinned = false;
    } catch (err) {
      error = String(err);
    }
  }

  async function openTodayNote(): Promise<void> {
    if (!(await flushSave())) return;
    error = '';
    try {
      const note = await OpenDailyNote();
      selected = note;
      lastSavedSnapshot = snapshot(note);
      saveState = 'clean';
      isCurrentPinned = await IsNotePinned(note.relativePath);
      await refresh();
    } catch (err) {
      error = String(err);
    }
  }

  async function togglePinCurrent(): Promise<void> {
    if (!selected) return;
    const path = selected.relativePath;
    const newState = !isCurrentPinned;
    try {
      await PinNote(path, newState);
      isCurrentPinned = newState;
      await refreshPinnedAndFolders();
      showToast('info', newState ? 'Note épinglée.' : 'Note désépinglée.');
    } catch (err) {
      showToast('error', `Échec : ${err}`);
    }
  }

  function onEditorChange(content: string): void {
    if (!selected) return;
    selected.content = content;
    selected = selected;
    scheduleAutoSave();
  }

  function onTitleChange(): void {
    if (!selected) return;
    selected = selected;
    scheduleAutoSave();
  }

  function scheduleAutoSave(): void {
    if (!selected) return;
    if (snapshot(selected) === lastSavedSnapshot) {
      saveState = 'clean';
      return;
    }
    saveState = 'dirty';
    if (autoSaveTimer) clearTimeout(autoSaveTimer);
    autoSaveTimer = setTimeout(() => {
      void flushSave();
    }, AUTO_SAVE_DEBOUNCE_MS);
  }

  async function flushSave(): Promise<boolean> {
    if (autoSaveTimer) {
      clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
    }
    if (!selected) return true;
    if (snapshot(selected) === lastSavedSnapshot && saveState === 'clean') {
      return true;
    }
    saveState = 'saving';
    error = '';
    try {
      const saved = await SaveNote(selected);
      selected = saved;
      lastSavedSnapshot = snapshot(saved);
      saveState = 'clean';
      lastSavedAt = new Date();
      await refreshPinnedAndFolders();
      return true;
    } catch (err) {
      saveState = 'error';
      const message = String(err);
      error = message;
      showToast('error', `Échec de l'enregistrement : ${message}`);
      return false;
    }
  }

  async function saveSelected(): Promise<void> {
    saving = true;
    try {
      await flushSave();
      await refresh();
    } finally {
      saving = false;
    }
  }

  function requestDelete(): void {
    if (!selected || deleting) return;
    confirmingDelete = true;
  }

  function cancelDelete(): void {
    if (deleting) return;
    confirmingDelete = false;
  }

  async function confirmDelete(): Promise<void> {
    if (!selected || deleting) return;
    if (autoSaveTimer) {
      clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
    }
    const relativePath = selected.relativePath;
    deleting = true;
    error = '';

    try {
      await DeleteNote(relativePath);
      if (selected?.relativePath === relativePath) {
        selected = null;
        lastSavedSnapshot = '';
        saveState = 'clean';
        lastSavedAt = null;
        isCurrentPinned = false;
      }
      confirmingDelete = false;
      await refresh();
      showToast('info', 'Note déplacée dans la corbeille.');
    } catch (err) {
      error = String(err);
    } finally {
      deleting = false;
    }
  }

  function toggleTheme(): void {
    theme = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = theme;
    window.localStorage.setItem('notevault-theme', theme);
  }

  function parseDateRange(value: string): { from?: Date; to?: Date } {
    const v = value.trim();
    const now = new Date();
    const today = (d: Date) =>
      new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
    if (v === 'today') {
      const start = today(now);
      return { from: new Date(start), to: new Date(start + 86400_000) };
    }
    if (v === 'yesterday') {
      const start = today(now) - 86400_000;
      return { from: new Date(start), to: new Date(start + 86400_000) };
    }
    if (v === 'thisweek') {
      const start = today(now) - now.getDay() * 86400_000;
      return { from: new Date(start), to: new Date(start + 7 * 86400_000) };
    }
    let cmp: 'g' | 'G' | 'L' | 'l' | '=' = '=';
    let rest = v;
    if (v.length > 1) {
      if (v[0] === '>' && v[1] === '=') {
        cmp = 'G';
        rest = v.slice(2);
      } else if (v[0] === '<' && v[1] === '=') {
        cmp = 'l';
        rest = v.slice(2);
      } else if (v[0] === '>') {
        cmp = 'g';
        rest = v.slice(1);
      } else if (v[0] === '<') {
        cmp = 'L';
        rest = v.slice(1);
      }
    }
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(rest);
    if (!m) return {};
    const d = new Date(
      Number(m[1]),
      Number(m[2]) - 1,
      Number(m[3])
    ).getTime();
    switch (cmp) {
      case 'G':
      case 'g':
        return { from: new Date(d) };
      case 'l':
        return { to: new Date(d + 86400_000) };
      case 'L':
        return { to: new Date(d) };
      default:
        return { from: new Date(d), to: new Date(d + 86400_000) };
    }
  }

  function parseFilter(input: string): {
    chips: { kind: string; text: string }[];
    fq: {
      Query: string;
      Tags: string[];
      ExcludeTags: string[];
      Path: string;
      UpdatedFrom?: string;
      UpdatedTo?: string;
    } | null;
  } {
    const trimmed = input.trim();
    if (!trimmed) {
      return { chips: [], fq: null };
    }
    const fq = {
      Query: '',
      Tags: [] as string[],
      ExcludeTags: [] as string[],
      Path: '',
      UpdatedFrom: undefined as string | undefined,
      UpdatedTo: undefined as string | undefined
    };
    const parts = trimmed.split(/\s+/);
    const plain: string[] = [];
    for (const part of parts) {
      if (part.startsWith('-tag:')) {
        const t = part.slice('-tag:'.length).replace(/^#/, '').trim();
        if (t) fq.ExcludeTags.push(t);
      } else if (part.startsWith('tag:')) {
        const t = part.slice('tag:'.length).replace(/^#/, '').trim();
        if (t) fq.Tags.push(t);
      } else if (part.startsWith('path:')) {
        let v = part.slice('path:'.length).trim();
        v = v.replace(/\/\*$/, '').replace(/\/$/, '');
        fq.Path = v;
      } else if (part.startsWith('updated:')) {
        const v = part.slice('updated:'.length).trim();
        const { from, to } = parseDateRange(v);
        if (from) fq.UpdatedFrom = from.toISOString();
        if (to) fq.UpdatedTo = to.toISOString();
      } else {
        plain.push(part);
      }
    }
    fq.Query = plain.join(' ');
    const chips: { kind: string; text: string }[] = [];
    fq.Tags.forEach((t) => chips.push({ kind: 'tag', text: t }));
    fq.ExcludeTags.forEach((t) => chips.push({ kind: 'exclude', text: '-' + t }));
    if (fq.Path) chips.push({ kind: 'path', text: fq.Path });
    if (fq.UpdatedFrom) chips.push({ kind: 'updatedFrom', text: '≥ ' + fq.UpdatedFrom.slice(0, 10) });
    if (fq.UpdatedTo) chips.push({ kind: 'updatedTo', text: '< ' + fq.UpdatedTo.slice(0, 10) });
    return { chips, fq };
  }

  function onFilterChange(value: string): void {
    activeFilter = value;
    const parsed = parseFilter(value);
    activeChips = parsed.chips;
    parsedFilter = parsed.fq;
    void refresh();
  }

  function onRemoveChip(kind: string, text: string): void {
    const current = parseFilter(activeFilter);
    let next = activeFilter;
    if (kind === 'tag') {
      next = activeFilter.replace(new RegExp(`(^|\\s)tag:${text}(\\s|$)`, 'g'), ' ').trim();
    } else if (kind === 'exclude') {
      const t = text.replace(/^-/, '');
      next = activeFilter.replace(new RegExp(`(^|\\s)-tag:${t}(\\s|$)`, 'g'), ' ').trim();
    } else if (kind === 'path') {
      next = activeFilter.replace(new RegExp(`(^|\\s)path:${text}(\\s|$)`, 'g'), ' ').trim();
    }
    onFilterChange(next);
  }

  function onClearFilter(): void {
    onFilterChange('');
  }

  // Navigation clavier dans la sidebar (j/k/Enter quand pas dans un input).
  function onSidebarKey(event: KeyboardEvent): void {
    const target = event.target as HTMLElement | null;
    if (target && ['INPUT', 'TEXTAREA'].includes(target.tagName)) return;
    if (target && target.isContentEditable) return;
    const list = activeChips.length > 0 || activeFilter ? notes : [...pinned, ...notes];
    if (event.key === 'j') {
      event.preventDefault();
      moveCursor(list, +1);
    } else if (event.key === 'k') {
      event.preventDefault();
      moveCursor(list, -1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      const path = selected?.relativePath;
      const entry = list.find((n) => n.relativePath === path) ?? list[0];
      if (entry) void openNote(entry.relativePath);
    } else if (event.key === 'h') {
      event.preventDefault();
      sidebarFocused = false;
    } else if (event.key === 'l') {
      event.preventDefault();
      sidebarFocused = false;
      const title = document.querySelector<HTMLInputElement>('input[aria-label="Titre de la note"]');
      title?.focus();
    }
  }

  function moveCursor(list: NoteSummary[], delta: number): void {
    if (list.length === 0) return;
    const idx = list.findIndex((n) => n.relativePath === selected?.relativePath);
    const next = idx < 0 ? 0 : Math.max(0, Math.min(list.length - 1, idx + delta));
    void openNote(list[next].relativePath);
  }

  function onGlobalKeydown(event: KeyboardEvent): void {
    const meta = event.ctrlKey || event.metaKey;
    const target = event.target as HTMLElement | null;
    const inEditable =
      target && ['INPUT', 'TEXTAREA'].includes(target.tagName);
    if (meta && event.shiftKey && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      void togglePinCurrent();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'f') {
      event.preventDefault();
      filterBar?.focus();
      return;
    }
    if (meta && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      quickSwitcherOpen = !quickSwitcherOpen;
      return;
    }
    if (meta && event.key.toLowerCase() === 's') {
      event.preventDefault();
      void saveSelected();
      return;
    }
    if (meta && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      void createNote();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'd') {
      event.preventDefault();
      void openTodayNote();
      return;
    }
    if (event.key === 'Escape' && quickSwitcherOpen) {
      quickSwitcherOpen = false;
      return;
    }
    if (!inEditable && (event.key === 'j' || event.key === 'k' || event.key === 'h' || event.key === 'l')) {
      onSidebarKey(event);
    }
  }

  function onSidebarFocus(): void {
    sidebarFocused = true;
  }

  function onBeforeUnload(event: BeforeUnloadEvent): void {
    if (saveState === 'dirty' || saveState === 'saving') {
      event.preventDefault();
      event.returnValue = '';
    }
  }

  function pickEntry(entry: { relativePath: string }): void {
    quickSwitcherOpen = false;
    void openNote(entry.relativePath);
  }

  void refresh();
</script>

<svelte:window onkeydown={onGlobalKeydown} onbeforeunload={onBeforeUnload} />

<div class="grid h-dvh min-h-0 grid-rows-[14rem_minmax(0,1fr)] bg-background text-foreground lg:grid-cols-[20rem_minmax(0,1fr)] lg:grid-rows-none">
  <aside
    bind:this={sidebarEl}
    class="flex min-h-0 flex-col border-b border-border bg-sidebar lg:border-b-0 lg:border-r"
    onfocusin={onSidebarFocus}
    aria-label="Navigation des notes"
  >
    <div class="flex h-14 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
      <div class="min-w-0">
        <h1 class="truncate text-sm font-semibold tracking-normal">NoteVault</h1>
        <p class="truncate text-xs text-subtle" title={vaultPath}>
          {vaultPath || 'Chargement du coffre...'}
        </p>
      </div>
      <span class="ml-2 rounded-md border border-border-strong px-2 py-0.5 text-xs text-subtle">
        {notes.length}
      </span>
    </div>

    <div class="flex min-h-0 flex-1 flex-col gap-2 px-3 py-3">
      <FilterBar
        bind:this={filterBar}
        value={activeFilter}
        chips={activeChips}
        onChange={onFilterChange}
        onRemoveChip={onRemoveChip}
        onClear={onClearFilter}
      />

      <div class="flex items-center justify-between gap-2 px-1">
        <div class="inline-flex items-center rounded-md border border-border bg-panel p-0.5">
          <button
            class={view === 'flat'
              ? 'inline-flex h-6 items-center gap-1 rounded px-2 text-xs font-medium text-foreground'
              : 'inline-flex h-6 items-center gap-1 rounded px-2 text-xs text-subtle hover:text-foreground'}
            type="button"
            title="Vue à plat"
            aria-pressed={view === 'flat'}
            onclick={() => setView('flat')}
          >
            <LayoutList size={11} strokeWidth={2} aria-hidden="true" />
            À plat
          </button>
          <button
            class={view === 'tree'
              ? 'inline-flex h-6 items-center gap-1 rounded px-2 text-xs font-medium text-foreground'
              : 'inline-flex h-6 items-center gap-1 rounded px-2 text-xs text-subtle hover:text-foreground'}
            type="button"
            title="Vue arborescente"
            aria-pressed={view === 'tree'}
            onclick={() => setView('tree')}
          >
            <FolderTree size={11} strokeWidth={2} aria-hidden="true" />
            Arborescence
          </button>
        </div>
        <div class="flex items-center gap-1">
          <button
            class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
            type="button"
            title="Recherche rapide (Ctrl+P)"
            aria-label="Recherche rapide"
            onclick={() => (quickSwitcherOpen = true)}
          >
            <Search size={13} strokeWidth={2} aria-hidden="true" />
          </button>
          <button
            class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
            type="button"
            title="Note du jour (Ctrl+Shift+D)"
            aria-label="Note du jour"
            onclick={() => openTodayNote()}
          >
            <CalendarDays size={13} strokeWidth={2} aria-hidden="true" />
          </button>
          <button
            class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
            type="button"
            title="Nouvelle note (Ctrl+N)"
            aria-label="Nouvelle note"
            onclick={() => createNote()}
          >
            <Plus size={13} strokeWidth={2} aria-hidden="true" />
          </button>
        </div>
      </div>

      {#if loading}
        <div class="flex flex-col gap-2" aria-busy="true">
          {#each [0, 1, 2, 3, 4] as _ (Math.random())}
            <div class="h-12 animate-pulse rounded-lg border border-border bg-panel-muted"></div>
          {/each}
        </div>
      {:else if notes.length === 0 && pinned.length === 0}
        <div class="rounded-lg border border-dashed border-border-strong bg-panel-muted px-3 py-3 text-sm leading-6 text-subtle">
          {#if activeFilter || activeChips.length > 0}
            Aucune note ne correspond aux filtres actifs.
          {:else}
            Aucune note. Créez-en une pour démarrer.
          {/if}
        </div>
      {:else}
        {#if pinned.length > 0 && !activeFilter && activeChips.length === 0}
          <section aria-label="Notes épinglées" class="flex flex-col gap-1">
            <h2 class="flex items-center gap-1 px-1 text-xs font-semibold uppercase text-subtle">
              <Pin size={11} strokeWidth={2.5} aria-hidden="true" />
              Épinglées
            </h2>
            <div class="flex flex-col gap-0.5 px-1">
              {#each pinned as note (note.relativePath)}
                {@const active = selected?.relativePath === note.relativePath}
                <button
                  type="button"
                  class={active
                    ? 'flex w-full items-center gap-1.5 rounded-md border border-accent bg-accent/15 px-2 py-1 text-left text-foreground'
                    : 'flex w-full items-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-left text-foreground hover:border-border hover:bg-panel-muted'}
                  aria-current={active ? 'page' : undefined}
                  onclick={() => openNote(note.relativePath)}
                >
                  <Pin size={11} strokeWidth={2.5} class="shrink-0 text-accent" aria-hidden="true" />
                  <span class="min-w-0 flex-1 truncate text-sm">{note.title || 'Sans titre'}</span>
                </button>
              {/each}
            </div>
          </section>
        {/if}

        {#if view === 'tree'}
          <section aria-label="Notes" class="min-h-0 flex-1 overflow-y-auto">
            <h2 class="sr-only">Notes</h2>
            <SidebarTree
              notes={notes}
              pinned={pinned}
              folders={folders}
              selectedPath={selectedPath}
              onOpen={openNote}
              onTogglePin={(p) => {
                if (selected?.relativePath === p) {
                  void togglePinCurrent();
                } else {
                  void PinNote(p, !pinnedSet.has(p)).then(() => refreshPinnedAndFolders());
                }
              }}
            />
          </section>
        {:else}
          <div class="min-h-0 flex-1">
            <VirtualList
              items={notes}
              itemHeight={56}
              overscan={6}
              class="h-full"
              ariaLabel="Liste des notes"
            >
              {#snippet children(note: NoteSummary)}
                {@const active = selected?.relativePath === note.relativePath}
                <button
                  type="button"
                  class={active
                    ? 'grid w-full gap-1 rounded-lg border border-accent bg-panel px-3 py-2 text-left shadow-sm'
                    : 'grid w-full gap-1 rounded-lg border border-transparent px-3 py-2 text-left hover:border-border hover:bg-panel-muted'}
                  aria-current={active ? 'page' : undefined}
                  onclick={() => openNote(note.relativePath)}
                >
                  <span class="flex min-w-0 items-center gap-1.5">
                    {#if pinnedSet.has(note.relativePath)}
                      <Pin size={11} strokeWidth={2.5} class="shrink-0 text-accent" aria-hidden="true" />
                    {/if}
                    <span class="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
                      {note.title || 'Sans titre'}
                    </span>
                  </span>
                  <span class="truncate text-xs text-subtle">
                    {formatDate(note.updatedAt)}
                    {#if note.relativePath.includes('/')}
                      · <span class="text-faint">{note.relativePath.split('/').slice(1, -1).join('/')}</span>
                    {/if}
                  </span>
                </button>
              {/snippet}
            </VirtualList>
          </div>
        {/if}
      {/if}
    </div>

    <div class="flex h-9 shrink-0 items-center justify-between border-t border-border px-3 text-xs text-subtle">
      <span>j/k · ↑/↓ &nbsp;Entrée ouvre</span>
      <span>{sidebarFocused ? 'sidebar active' : 'éditeur actif'}</span>
    </div>
  </aside>

  <main class="grid min-h-0 grid-rows-[3.5rem_minmax(0,1fr)] bg-background">
    <header class="flex min-w-0 items-center justify-between gap-3 border-b border-border bg-panel px-4">
      <div class="min-w-0">
        <nav class="flex min-w-0 items-center gap-2 text-xs text-subtle" aria-label="Fil d'Ariane">
          <span class="shrink-0">NoteVault</span>
          <span aria-hidden="true">/</span>
          <span class="truncate text-foreground">{currentTitle}</span>
        </nav>
        <p class="mt-0.5 truncate text-xs text-faint" title={selectedPath || vaultPath}>
          {selectedPath || vaultPath || 'Coffre local'}
        </p>
      </div>

      <div class="flex items-center gap-2">
        {#if selected}
          <button
            class={isCurrentPinned
              ? 'inline-flex h-9 items-center gap-1 rounded-md border border-accent bg-accent/15 px-2.5 text-xs font-medium text-accent hover:bg-accent/20'
              : 'inline-flex h-9 items-center gap-1 rounded-md border border-border bg-panel-muted px-2.5 text-xs text-subtle hover:bg-sidebar hover:text-foreground'}
            type="button"
            title={isCurrentPinned ? 'Désépingler (Ctrl+Shift+P)' : 'Épingler (Ctrl+Shift+P)'}
            aria-pressed={isCurrentPinned}
            onclick={() => togglePinCurrent()}
          >
            {#if isCurrentPinned}
              <PinOff size={14} strokeWidth={2} aria-hidden="true" />
              Épinglée
            {:else}
              <Pin size={14} strokeWidth={2} aria-hidden="true" />
              Épingler
            {/if}
          </button>
        {/if}
        <button
          class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
          type="button"
          title={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
          aria-label={theme === 'dark' ? 'Passer au thème clair' : 'Passer au thème sombre'}
          aria-pressed={theme === 'dark'}
          onclick={toggleTheme}
        >
          {#if theme === 'dark'}
            <Sun size={17} strokeWidth={2} aria-hidden="true" />
          {:else}
            <Moon size={17} strokeWidth={2} aria-hidden="true" />
          {/if}
        </button>
      </div>
    </header>

    <section class="flex min-h-0 flex-col overflow-hidden" aria-label="Éditeur de note">
      {#if error}
        <p class="mx-4 mt-4 rounded-lg border border-danger/40 bg-panel px-3 py-2 text-sm text-danger" role="alert">
          {error}
        </p>
      {/if}

      {#if selected}
        <div class="flex min-h-0 flex-1 flex-col">
          <input
            class="block w-full shrink-0 border-0 bg-transparent px-4 pb-3 pt-5 text-3xl font-semibold leading-tight text-foreground outline-none placeholder:text-faint focus:outline-none focus-visible:outline-none sm:text-4xl"
            aria-label="Titre de la note"
            bind:value={selected.title}
            oninput={onTitleChange}
            placeholder="Sans titre"
          />

          <div class="min-h-0 flex-1">
            <NoteEditor markdown={selected.content} onChange={onEditorChange} />
          </div>

          <footer class="flex min-h-12 items-center justify-between gap-3 border-t border-border bg-panel px-4 text-xs text-faint">
            <div class="flex min-w-0 items-center gap-3">
              <SaveIndicator state={saveState} lastSavedAt={lastSavedAt} />
              {#if hasUnsavedChanges && saveState !== 'saving'}
                <span class="text-faint">modifications en attente…</span>
              {/if}
            </div>
            <div class="flex min-w-0 items-center gap-3">
              <span class="truncate" title={selected.relativePath}>{selected.relativePath}</span>
              <div class="flex shrink-0 items-center gap-2">
                <button
                  class="inline-flex items-center gap-2 rounded-md border border-danger/45 bg-transparent px-3 py-1.5 text-sm font-medium text-danger hover:bg-danger/10 disabled:hover:bg-transparent"
                  type="button"
                  onclick={requestDelete}
                  disabled={deleting || saving}
                >
                  <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
                  Supprimer
                </button>
                <button
                  class="inline-flex items-center gap-2 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover disabled:hover:bg-accent"
                  type="button"
                  onclick={saveSelected}
                  disabled={saving || deleting}
                >
                  <Save size={15} strokeWidth={2} aria-hidden="true" />
                  {saving ? 'Enregistrement...' : 'Enregistrer'}
                </button>
              </div>
            </div>
          </footer>
        </div>
      {:else}
        <div class="grid min-h-0 flex-1 place-items-center rounded-lg border border-dashed border-border-strong bg-panel-muted p-8 text-center">
          <div class="max-w-md">
            <h2 class="text-2xl font-semibold text-foreground">Vos notes vous appartiennent.</h2>
            <p class="mt-2 text-sm leading-6 text-subtle">
              Les fichiers Markdown sont enregistrés dans votre coffre local.
            </p>
            <p class="mt-4 text-xs text-faint">
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+P</kbd>
              recherche rapide ·
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+Shift+F</kbd>
              filtres ·
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+Shift+P</kbd>
              épingler
            </p>
          </div>
        </div>
      {/if}
    </section>
  </main>
</div>

<QuickSwitcher
  open={quickSwitcherOpen}
  entries={allEntries}
  onPick={pickEntry}
  onClose={() => (quickSwitcherOpen = false)}
/>

{#if toast}
  <div class="pointer-events-none fixed bottom-6 right-6 z-40 flex max-w-sm flex-col gap-2">
    <div
      class="pointer-events-auto rounded-md border px-3 py-2 text-sm shadow-lg {toast.kind === 'error' ? 'border-danger/50 bg-panel text-danger' : 'border-border bg-panel text-foreground'}"
      role="status"
    >
      {toast.message}
    </div>
  </div>
{/if}

{#if confirmingDelete && selected}
  <div class="fixed inset-0 z-50 grid place-items-center px-4">
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer la confirmation"
      onclick={cancelDelete}
      disabled={deleting}
    ></button>
    <div
      class="relative w-full max-w-sm rounded-lg border border-border bg-panel p-4 shadow-xl"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-note-title"
    >
      <h2 id="delete-note-title" class="text-base font-semibold text-foreground">
        Supprimer cette note ?
      </h2>
      <p class="mt-2 text-sm leading-6 text-subtle">
        Cette action déplacera le fichier dans la corbeille.
      </p>
      <p class="mt-3 truncate rounded-md bg-background px-2 py-1 text-xs text-faint" title={selected.relativePath}>
        {selected.relativePath}
      </p>
      <div class="mt-4 flex justify-end gap-2">
        <button
          class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          onclick={cancelDelete}
          disabled={deleting}
        >
          Annuler
        </button>
        <button
          class="inline-flex items-center gap-2 rounded-md border border-danger/45 bg-danger px-3 py-1.5 text-sm font-medium text-background hover:opacity-90"
          type="button"
          onclick={confirmDelete}
          disabled={deleting}
        >
          <Trash2 size={15} strokeWidth={2} aria-hidden="true" />
          {deleting ? 'Suppression...' : 'Supprimer'}
        </button>
      </div>
    </div>
  </div>
{/if}