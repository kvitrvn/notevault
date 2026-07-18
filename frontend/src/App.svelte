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
  import Hash from '@lucide/svelte/icons/hash';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Copy from '@lucide/svelte/icons/copy';
  import CopyPlus from '@lucide/svelte/icons/copy-plus';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import FolderInput from '@lucide/svelte/icons/folder-input';
  import GripVertical from '@lucide/svelte/icons/grip-vertical';
  import History from '@lucide/svelte/icons/history';
  import Cloud from '@lucide/svelte/icons/cloud';
  import Activity from '@lucide/svelte/icons/activity';
  import Download from '@lucide/svelte/icons/download';
  import Keyboard from '@lucide/svelte/icons/keyboard';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import PanelLeftClose from '@lucide/svelte/icons/panel-left-close';
  import PanelLeftOpen from '@lucide/svelte/icons/panel-left-open';
  import MessageSquare from '@lucide/svelte/icons/message-square';

  import NoteEditor from './components/NoteEditor.svelte';
  import SaveIndicator from './components/SaveIndicator.svelte';
  import VirtualList from './components/VirtualList.svelte';
  import QuickSwitcher from './components/QuickSwitcher.svelte';
  import FilterBar from './components/FilterBar.svelte';
  import SidebarTree from './components/SidebarTree.svelte';
  import TagEditor from './components/TagEditor.svelte';
  import ContextMenu from './components/ContextMenu.svelte';
  import TagsView from './components/TagsView.svelte';
  import TemplatePickerDialog from './components/TemplatePickerDialog.svelte';
  import MoveDialog from './components/MoveDialog.svelte';
  import BacklinksPanel from './components/BacklinksPanel.svelte';
  import HistoryPanel from './components/HistoryPanel.svelte';
  import VaultPickerDialog from './components/VaultPickerDialog.svelte';
  import OnboardingModal from './components/OnboardingModal.svelte';
  import ShortcutsOverlay from './components/ShortcutsOverlay.svelte';
  import ThemeMenu from './components/ThemeMenu.svelte';
  import StatsView from './components/StatsView.svelte';
  import ExportDialog from './components/ExportDialog.svelte';
  import RecoveryDialog from './components/RecoveryDialog.svelte';
  import WindowTitleBar from './components/WindowTitleBar.svelte';
  import ChatPanel from './components/ChatPanel.svelte';
  import type { SaveState } from './components/SaveIndicator.svelte';
  import { isLocalAssetPath, precomputeAssetURLs as resolveAssetURLs } from './lib/assets';
  import { shouldShowVaultUnlock } from './lib/vault-manager';
  import { domain, vault } from '../wailsjs/go/models';

  import {
    ApplicationStatus,
    ClearDirtyBuffer,
    CreateVault,
    CreateNote,
    DeleteNote,
    DuplicateNote,
    EnsureDailyNote,
    GetBacklinks,
    GetConfig,
    IsNotePinned,
    ListFolders,
    ListNotes,
    ListNotesFiltered,
    ListPinned,
    ListTags,
    ListTemplates,
    ListThemes,
    MoveNote,
    OpenDailyNote,
    OpenInExplorer,
    OpenNote,
    PinNote,
    RenameTitle,
    RestoreFromHistory,
    SaveAsset,
    SaveNote,
    SearchNotes,
    SetDirtyBuffer,
    SnapshotForStartup,
    UpdateConfig,
    VaultPath,
    AssetURL,
    ImportAssetFromFilePath,
    ForgetRecentVault,
    OpenVault,
    VaultStatus,
    UnlockVault,
    EnableEncryption,
    ChangePassphrase,
    DisableEncryption
  } from '../wailsjs/go/main/App';

  type Note = domain.Note;
  type NoteSummary = domain.NoteSummary;
  // Reconstruit une `Note` typée à partir d'un plain object (sans perdre
  // la méthode convertValues requise par le type généré par Wails).
  function cloneNote(note: Note, content: string | undefined): Note {
    return domain.Note.createFrom({ ...note, content: content ?? note.content });
  }
  type FolderInfo = vault.FolderInfo;
  type FilterQuery = vault.FilterQuery;
  type TagCount = vault.TagCount;
  type Template = vault.Template;
  type ContextMenuItem = {
    label: string;
    icon?: typeof Pencil;
    danger?: boolean;
    onPick: () => void;
  };

  const AUTO_SAVE_DEBOUNCE_MS = 1500;

  let notes: NoteSummary[] = $state([]);
  let chatNotes: NoteSummary[] = $state([]);
  let pinned: NoteSummary[] = $state([]);
  let folders: FolderInfo[] = $state([]);
  let tags: TagCount[] = $state([]);
  let templates: Template[] = $state([]);
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

  let theme = $state<'dark' | 'light'>(
    document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'
  );
  let activeThemeId = $state<string>(
    (document.documentElement.dataset.theme as 'dark' | 'light') || 'dark'
  );

  // Vue sidebar
  let view: 'flat' | 'tree' = $state(
    (window.localStorage.getItem('notevault-view') as 'flat' | 'tree') || 'flat'
  );
  let sidebarFocused = $state(false);
  let sidebarCollapsed = $state(window.localStorage.getItem('notevault-sidebar-collapsed') === '1');
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

  // Tags view
  let tagsViewOpen = $state(false);

  // Template picker (Cmd+N)
  let templatePickerOpen = $state(false);

  // Move dialog
  let moveDialogOpen = $state(false);
  let moveTarget = $state('');
  let foldersLoaded = $state(false);
  let foldersLoading = $state(false);

  // History panel
  let historyOpen = $state(false);

  // Vault picker
  let vaultPickerOpen = $state(false);
  let vaultMenuOpen = $state(false);
  let applicationStatus = $state<domain.ApplicationStatus | null>(null);
  let vaultSwitching = $state(false);
  let vaultSwitchError = $state('');

  // Phase 5 : onboarding, raccourcis, stats, export, recovery
  let onboardingOpen = $state(false);
  let shortcutsOpen = $state(false);
  let statsOpen = $state(false);
  let exportOpen = $state(false);
  let chatOpen = $state(false);
  let recoverySnapshot = $state<vault.RecoverySnapshot | null>(null);
  let recoveryOpen = $state(false);
  let customThemes = $state<vault.Theme[]>([]);
  let dirtyBufferTimer: ReturnType<typeof setTimeout> | null = null;
  let onboardingShownThisLaunch = false;

  let vaultStatus = $state<vault.VaultStatusInfo | null>(null);
  let unlockPassphrase = $state('');
  let unlockInput: HTMLInputElement | undefined = $state();
  let unlockError = $state('');
  let unlocking = $state(false);
  let encryptionDialogOpen = $state(false);
  let encryptionAction = $state<'enable' | 'change' | 'disable'>('enable');
  let currentPassphrase = $state('');
  let replacementPassphrase = $state('');
  let confirmationPassphrase = $state('');
  let encryptionError = $state('');
  let encryptionBusy = $state(false);

  $effect(() => {
    if (vaultStatus?.state === 'locked') {
      requestAnimationFrame(() => unlockInput?.focus());
    }
  });

  // Vault sync awareness
  let vaultIsSynced = $state(false);

  // Known titles for wiki-link resolution
  const knownTitles = $derived(new Set(notes.map((n) => n.title).filter(Boolean)));

  // Pin state
  let isCurrentPinned = $state(false);

  // Context menu (right-click on sidebar item)
  let contextMenu = $state<{ x: number; y: number; items: ContextMenuItem[] } | null>(null);

  // Inline rename state
  let titleEditing = $state(false);
  let titleDraft = $state('');

  // Drag state
  let dragSource = $state<string | null>(null);
  let dragOverFolder = $state<string | null>(null);

  let filterBar = $state<FilterBar>();
  let sidebarEl: HTMLElement | undefined = $state();
  let titleEl: HTMLInputElement | undefined = $state();
  let noteEditor:
    | {
        flushPendingChange: () => void;
      }
    | undefined = $state();

  const currentTitle = $derived(selected?.title?.trim() || 'Aucune note');
  const selectedPath = $derived(selected?.relativePath || '');
  const hasUnsavedChanges = $derived(
    selected !== null && snapshot(selected) !== lastSavedSnapshot
  );
  const pinnedSet = $derived(new Set(pinned.map((p) => p.relativePath)));
  const isDev = Boolean((import.meta as ImportMeta & { env?: { DEV?: boolean } }).env?.DEV);
  let refreshSeq = 0;
  let perfSeq = 0;

  function startPerf(label: string): string {
    if (!isDev) return '';
    const id = `${label}:${++perfSeq}`;
    console.time(id);
    return id;
  }

  function endPerf(id: string): void {
    if (!id || !isDev) return;
    console.timeEnd(id);
  }

  function scheduleIdle(task: () => void): void {
    const win = window as Window & {
      requestIdleCallback?: (callback: () => void, options?: { timeout: number }) => number;
    };
    if (typeof win.requestIdleCallback === 'function') {
      win.requestIdleCallback(task, { timeout: 1000 });
      return;
    }
    window.setTimeout(task, 0);
  }

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

  function toggleSidebar(): void {
    sidebarCollapsed = !sidebarCollapsed;
    window.localStorage.setItem('notevault-sidebar-collapsed', sidebarCollapsed ? '1' : '0');
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

  // Wrapper safeCall : timeout + log pour identifier les requêtes qui
  // bloquent. Chaque appel est isolé : un échec n'empêche pas les autres.
  // Svelte ne supporte pas les génériques en inline, on utilise un cast any.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const safeCall = async (label: string, p: Promise<any>, fallback: any): Promise<any> => {
    let to: ReturnType<typeof setTimeout> | undefined;
    const timeout = new Promise<any>((resolve) => {
      to = setTimeout(() => {
        console.warn(`[refresh] ${label} timeout`);
        resolve(fallback);
      }, 8000);
    });
    try {
      const result = await Promise.race([p, timeout]);
      return result;
    } catch (err) {
      console.error(`[refresh] ${label} failed:`, err);
      return fallback;
    } finally {
      if (to) clearTimeout(to);
    }
  };

  async function refresh(): Promise<void> {
    const seq = ++refreshSeq;
    const criticalPerf = startPerf('refresh:critical');
    loading = true;
    error = '';

    try {
      const cfg = await safeCall('GetConfig', GetConfig(), { autoDailyNote: false });
      const fq = buildQuery();
      const fetchAll = !activeFilter.trim() && activeChips.length === 0;
      const [list, pin, vp] = await Promise.all([
        safeCall('ListNotes', fetchAll ? ListNotes() : ListNotesFiltered(fq, 1000), []),
        safeCall('ListPinned', ListPinned(), []),
        safeCall('VaultPath', VaultPath(), '')
      ]);
      if (seq !== refreshSeq) return;
      notes = (list ?? []) as NoteSummary[];
      pinned = (pin ?? []) as NoteSummary[];
      vaultPath = vp ?? '';
      if (fetchAll) {
        chatNotes = (list ?? []) as NoteSummary[];
        allEntries = ((list ?? []) as NoteSummary[]).map((n) => ({
          relativePath: n.relativePath,
          title: n.title,
          updatedAt: String(n.updatedAt ?? ''),
          score: 0
        }));
      }
      // Si la config demande un thème custom qui n'est pas le défaut, on l'applique.
      const cfgTheme = String((cfg as { theme?: string } | null)?.theme ?? 'dark');
      if (cfgTheme === 'light' || cfgTheme === 'dark') {
        applyThemeLocally(cfgTheme);
      }
      loading = false;
      scheduleIdle(() => void refreshDeferred(seq, cfg, fetchAll));
    } catch (err) {
      error = String(err);
      console.error('[refresh] global error:', err);
    } finally {
      if (seq === refreshSeq) loading = false;
      endPerf(criticalPerf);
    }
  }

  async function refreshDeferred(seq: number, cfg: unknown, fetchAll: boolean): Promise<void> {
    const deferredPerf = startPerf('refresh:deferred');
    try {
      const [tpl, tg, themes, all] = await Promise.all([
        safeCall('ListTemplates', ListTemplates(), []),
        safeCall('ListTags', ListTags(), []),
        safeCall('ListThemes', ListThemes(), []),
        fetchAll ? Promise.resolve(null) : safeCall('ListNotes (allEntries)', ListNotes(), [])
      ]);
      if (seq !== refreshSeq) return;
      templates = (tpl ?? []) as Template[];
      tags = (tg ?? []) as TagCount[];
      customThemes = (themes ?? []) as vault.Theme[];
      if (!fetchAll) {
        chatNotes = (all ?? []) as NoteSummary[];
        allEntries = ((all ?? []) as NoteSummary[]).map((n) => ({
          relativePath: n.relativePath,
          title: n.title,
          updatedAt: String(n.updatedAt ?? ''),
          score: 0
        }));
      }
      const cfgTheme = String((cfg as { theme?: string } | null)?.theme ?? 'dark');
      if (cfgTheme && cfgTheme !== 'dark' && cfgTheme !== 'light') {
        applyThemeLocally(cfgTheme, customThemes);
      }
      if ((cfg as { autoDailyNote?: boolean } | null)?.autoDailyNote === true) {
        try {
          await safeCall('EnsureDailyNote', EnsureDailyNote(), '');
          invalidateFolders();
        } catch {
          /* non bloquant */
        }
      }
    } finally {
      endPerf(deferredPerf);
    }
  }

  async function refreshPinnedAndTags(): Promise<void> {
    try {
      const [pin, tg] = await Promise.all([
        safeCall('ListPinned', ListPinned(), []),
        safeCall('ListTags', ListTags(), [])
      ]);
      pinned = pin as NoteSummary[];
      tags = tg as TagCount[];
    } catch {
      /* silencieux */
    }
  }

  function invalidateFolders(): void {
    foldersLoaded = false;
  }

  async function loadFolders(force = false): Promise<void> {
    if (foldersLoading || (foldersLoaded && !force)) return;
    const foldersPerf = startPerf('ListFolders:lazy');
    foldersLoading = true;
    try {
      const fold = await safeCall('ListFolders', ListFolders(), []);
      folders = (fold ?? []) as FolderInfo[];
      foldersLoaded = true;
    } finally {
      foldersLoading = false;
      endPerf(foldersPerf);
    }
  }

  async function openNote(relativePath: string): Promise<void> {
    if (!(await flushSave())) return;
    error = '';
    try {
      const note = await safeCall('OpenNote', OpenNote(relativePath), null);
      if (!note) {
        error = `Impossible d'ouvrir ${relativePath}`;
        return;
      }
      // Pré-transforme les chemins relatifs d'images en URLs absolues pour
      // que l'éditeur Tiptap puisse les charger dans la webview.
      const content = await precomputeAssetURLs(note.content);
      selected = cloneNote(note, content);
      lastSavedSnapshot = snapshot(selected!);
      saveState = 'clean';
      isCurrentPinned = await safeCall('IsNotePinned', IsNotePinned(relativePath), false);
    } catch (err) {
      error = String(err);
    }
  }

  // Transforme les assets du coffre en URLs loopback pour Tiptap. La politique
  // de chemin et le traitement Markdown vivent dans un utilitaire testé.
  async function precomputeAssetURLs(md: string): Promise<string> {
    return resolveAssetURLs(md, assetURL);
  }

  function openTemplatePicker(): void {
    templatePickerOpen = true;
  }

  async function createNoteFromTemplate(templateId: string, title: string): Promise<void> {
    templatePickerOpen = false;
    if (!(await flushSave())) return;
    error = '';
    try {
      const note = await CreateNote(title, templateId);
      const content = await precomputeAssetURLs(note.content);
      selected = cloneNote(note, content);
      lastSavedSnapshot = snapshot(selected!);
      saveState = 'clean';
      lastSavedAt = new Date();
      isCurrentPinned = false;
      invalidateFolders();
      await refresh();
    } catch (err) {
      error = String(err);
    }
  }

  async function openTodayNote(): Promise<void> {
    if (!(await flushSave())) return;
    error = '';
    try {
      const note = await OpenDailyNote();
      const content = await precomputeAssetURLs(note.content);
      selected = cloneNote(note, content);
      lastSavedSnapshot = snapshot(selected!);
      saveState = 'clean';
      isCurrentPinned = await IsNotePinned(note.relativePath);
      invalidateFolders();
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
      await refreshPinnedAndTags();
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

  function onEditorDirty(): void {
    if (!selected) return;
    saveState = 'dirty';
    scheduleAutoSave({ compareSnapshot: false });
  }

  function onTitleChange(): void {
    if (!selected) return;
    selected = selected;
    scheduleAutoSave();
  }

  function onTagsChange(next: string[]): void {
    if (!selected) return;
    selected.tags = next;
    selected = selected;
    chatNotes = chatNotes.map((note) =>
      note.relativePath === selected?.relativePath
        ? domain.NoteSummary.createFrom({ ...note, tags: next })
        : note
    );
    scheduleAutoSave();
  }

  function clearSaveTimers(): void {
    if (autoSaveTimer) {
      clearTimeout(autoSaveTimer);
      autoSaveTimer = null;
    }
    if (dirtyBufferTimer) {
      clearTimeout(dirtyBufferTimer);
      dirtyBufferTimer = null;
    }
  }

  function scheduleAutoSave(options: { compareSnapshot?: boolean } = {}): void {
    if (!selected) return;
    if (options.compareSnapshot !== false && snapshot(selected) === lastSavedSnapshot) {
      saveState = 'clean';
      clearSaveTimers();
      return;
    }
    saveState = 'dirty';
    if (autoSaveTimer) clearTimeout(autoSaveTimer);
    autoSaveTimer = setTimeout(() => {
      void flushSave();
    }, AUTO_SAVE_DEBOUNCE_MS);
    scheduleDirtyBuffer();
  }

  function scheduleDirtyBuffer(): void {
    if (!selected) return;
    if (dirtyBufferTimer) clearTimeout(dirtyBufferTimer);
    dirtyBufferTimer = setTimeout(() => {
      void persistDirtyBuffer();
    }, 5000);
  }

  async function persistDirtyBuffer(): Promise<void> {
    if (!selected) return;
    noteEditor?.flushPendingChange();
    if (snapshot(selected) === lastSavedSnapshot) {
      return;
    }
    try {
      // Le Go traite diskMTime vide comme "non défini" ; on envoie donc
      // une string ISO de l'époque UNIX (équivalent du zéro time Go).
      await SetDirtyBuffer(
        selected.relativePath,
        selected.content,
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        '0001-01-01T00:00:00Z' as any
      );
    } catch (err) {
      console.error('[recovery] persist failed:', err);
    }
  }

  async function flushSave(): Promise<boolean> {
    noteEditor?.flushPendingChange();
    clearSaveTimers();
    if (!selected) return true;
    if (snapshot(selected) === lastSavedSnapshot && saveState === 'clean') {
      return true;
    }
    saveState = 'saving';
    error = '';
    const noteToSave = domain.Note.createFrom({ ...selected });
    const saveSnapshot = snapshot(noteToSave);
    const savePath = noteToSave.relativePath;
    try {
      const saved = await SaveNote(noteToSave);
      const currentSnapshot = selected?.relativePath === savePath ? snapshot(selected) : '';
      const changedDuringSave = currentSnapshot !== '' && currentSnapshot !== saveSnapshot;

      if (!changedDuringSave) {
        selected = saved;
      }
      lastSavedSnapshot = snapshot(saved);
      saveState = changedDuringSave ? 'dirty' : 'clean';
      lastSavedAt = new Date();
      if (changedDuringSave) {
        scheduleAutoSave({ compareSnapshot: false });
      } else {
        try {
          await ClearDirtyBuffer();
        } catch (err) {
          console.error('[recovery] clear failed:', err);
        }
      }
      await refreshPinnedAndTags();
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

  // --- Inline rename -------------------------------------------------------
  function startRename(): void {
    if (!selected) return;
    titleDraft = selected.title;
    titleEditing = true;
    requestAnimationFrame(() => titleEl?.focus());
  }

  async function commitRename(): Promise<void> {
    titleEditing = false;
    if (!selected) return;
    const next = titleDraft.trim();
    if (next === selected.title.trim()) return;
    try {
      const updated = await RenameTitle(selected.relativePath, next);
      selected = updated;
      lastSavedSnapshot = snapshot(updated);
      saveState = 'clean';
      lastSavedAt = new Date();
      await refresh();
    } catch (err) {
      showToast('error', `Échec du renommage : ${err}`);
    }
  }

  function cancelRename(): void {
    titleEditing = false;
    titleDraft = selected?.title ?? '';
  }

  // --- Move / duplicate / explorer / context menu -------------------------
  function openMoveDialog(): void {
    if (!selected) return;
    moveTarget = selected.relativePath;
    moveDialogOpen = true;
    void loadFolders();
  }

  async function moveTo(newFolder: string): Promise<void> {
    moveDialogOpen = false;
    if (!selected) return;
    if (!(await flushSave())) return;
    const base = selected.relativePath.split('/').pop() ?? 'note.md';
    const target = newFolder + base;
    if (target === selected.relativePath) return;
    try {
      const moved = await MoveNote(selected.relativePath, target);
      showToast('info', `Note déplacée vers ${newFolder}`);
      invalidateFolders();
      await refresh();
      // Sélectionne la note déplacée.
      await openNote(moved.relativePath);
    } catch (err) {
      showToast('error', `Échec du déplacement : ${err}`);
    }
  }

  async function duplicateCurrent(): Promise<void> {
    if (!selected) return;
    if (!(await flushSave())) return;
    try {
      const dup = await DuplicateNote(selected.relativePath);
      showToast('info', 'Note dupliquée.');
      invalidateFolders();
      await refresh();
      await openNote(dup.relativePath);
    } catch (err) {
      showToast('error', `Échec de la duplication : ${err}`);
    }
  }

  async function copyCurrentPath(): Promise<void> {
    if (!selected) return;
    try {
      await navigator.clipboard.writeText(selected.relativePath);
      showToast('info', 'Chemin copié dans le presse-papiers.');
    } catch {
      showToast('error', 'Impossible de copier le chemin.');
    }
  }

  async function openInExplorerCurrent(): Promise<void> {
    if (!selected) return;
    try {
      await OpenInExplorer(selected.relativePath, true);
    } catch (err) {
      showToast('error', `Impossible d'ouvrir le dossier : ${err}`);
    }
  }

  function openContextMenu(event: MouseEvent, relPath: string): void {
    event.preventDefault();
    if (!selected || selected.relativePath !== relPath) {
      void openNote(relPath).then(() => buildContextMenu(event));
    } else {
      buildContextMenu(event);
    }
  }

  function buildContextMenu(event: MouseEvent): void {
    if (!selected) return;
    contextMenu = {
      x: event.clientX,
      y: event.clientY,
      items: [
        {
          label: 'Renommer le titre',
          icon: Pencil,
          onPick: () => startRename()
        },
        {
          label: 'Déplacer vers…',
          icon: FolderInput,
          onPick: () => openMoveDialog()
        },
        {
          label: 'Dupliquer',
          icon: CopyPlus,
          onPick: () => void duplicateCurrent()
        },
        {
          label: 'Copier le chemin',
          icon: Copy,
          onPick: () => void copyCurrentPath()
        },
        {
          label: 'Ouvrir dans le Finder',
          icon: ExternalLink,
          onPick: () => void openInExplorerCurrent()
        }
      ]
    };
  }

  // --- Drag & drop ---------------------------------------------------------
  function onDragStart(event: DragEvent, relPath: string): void {
    if (!event.dataTransfer) return;
    dragSource = relPath;
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('text/plain', relPath);
  }

  function onDragEnd(): void {
    dragSource = null;
    dragOverFolder = null;
  }

  function onFolderDragOver(event: DragEvent, folder: string): void {
    if (!dragSource) return;
    event.preventDefault();
    event.dataTransfer && (event.dataTransfer.dropEffect = 'move');
    dragOverFolder = folder;
  }

  function onFolderDragLeave(folder: string): void {
    if (dragOverFolder === folder) dragOverFolder = null;
  }

  async function onFolderDrop(event: DragEvent, folder: string): Promise<void> {
    event.preventDefault();
    const src = dragSource ?? event.dataTransfer?.getData('text/plain') ?? '';
    dragOverFolder = null;
    dragSource = null;
    if (!src || !src.startsWith('notes/')) return;
    const base = src.split('/').pop() ?? 'note.md';
    const targetFolder = (folder.startsWith('notes/') ? folder : 'notes/' + folder).replace(/\/+$/, '');
    const target = targetFolder + '/' + base;
    if (target === src) return;
    try {
      await MoveNote(src, target);
      showToast('info', `Note déplacée vers ${targetFolder}/`);
      invalidateFolders();
      await refresh();
    } catch (err) {
      showToast('error', `Échec : ${err}`);
    }
  }

  // --- Delete --------------------------------------------------------------
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
      invalidateFolders();
      await refresh();
      showToast('info', 'Note déplacée dans la corbeille.');
    } catch (err) {
      error = String(err);
    } finally {
      deleting = false;
    }
  }

  function toggleTheme(): void {
    const next = theme === 'dark' ? 'light' : 'dark';
    void selectTheme(next);
  }

  function applyThemeLocally(id: string, availableThemes: vault.Theme[] = customThemes): void {
    activeThemeId = id;
    const found = availableThemes.find((t) => t.id === id);
    if (id === 'dark' || id === 'light') {
      theme = id as 'dark' | 'light';
      document.documentElement.dataset.theme = theme;
    } else if (found) {
      // Pour les thèmes custom, on garde le set de variables dark/light
      // défini dans styles.css (couleurs de base) et on surcharge les vars.
      document.documentElement.dataset.theme = 'dark';
    }
    applyThemeVars(found?.vars ?? null);
    window.localStorage.setItem('notevault-theme', id);
  }

  async function selectTheme(id: string): Promise<void> {
    applyThemeLocally(id);
    try {
      const cfg = await GetConfig();
      if (cfg.theme !== id) {
        await UpdateConfig({ ...cfg, theme: id });
      }
    } catch (err) {
      console.error('[theme] persist failed:', err);
    }
  }

  function applyThemeVars(vars: Record<string, string> | null): void {
    const root = document.documentElement;
    // On retire d'abord les variables précédemment injectées.
    for (const key of Array.from(root.style)) {
      if (key.startsWith('--color-') && !key.endsWith('-inline')) {
        root.style.removeProperty(key);
      }
    }
    if (!vars) return;
    for (const [key, value] of Object.entries(vars)) {
      root.style.setProperty(key, value);
    }
  }

  // --- Filter parser -------------------------------------------------------
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
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3])).getTime();
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
    if (!trimmed) return { chips: [], fq: null };
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

  // --- Keyboard navigation -------------------------------------------------
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
      titleEl?.focus();
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
    const inEditable = target && ['INPUT', 'TEXTAREA'].includes(target.tagName);
    if (event.key === 'Escape') {
      if (contextMenu) {
        contextMenu = null;
        return;
      }
      if (quickSwitcherOpen) {
        quickSwitcherOpen = false;
        return;
      }
    }
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
    if (meta && event.shiftKey && event.key.toLowerCase() === 'r') {
      event.preventDefault();
      startRename();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'm') {
      event.preventDefault();
      openMoveDialog();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'd') {
      event.preventDefault();
      void openTodayNote();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'h') {
      event.preventDefault();
      openHistory();
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'b') {
      event.preventDefault();
      toggleSidebar();
      return;
    }
    if (meta && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      quickSwitcherOpen = !quickSwitcherOpen;
      return;
    }
    if (meta && event.key.toLowerCase() === 't') {
      event.preventDefault();
      tagsViewOpen = !tagsViewOpen;
      return;
    }
    if (meta && event.key.toLowerCase() === 's') {
      event.preventDefault();
      void saveSelected();
      return;
    }
    if (meta && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      openTemplatePicker();
      return;
    }
    if (meta && event.key === '/') {
      event.preventDefault();
      shortcutsOpen = !shortcutsOpen;
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'g') {
      event.preventDefault();
      statsOpen = !statsOpen;
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'e') {
      event.preventDefault();
      exportOpen = !exportOpen;
      return;
    }
    if (meta && event.shiftKey && event.key.toLowerCase() === 'c') {
      event.preventDefault();
      void openChat();
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

  async function onWindowClose(): Promise<boolean> {
    return flushSave();
  }

  function pickEntry(entry: { relativePath: string }): void {
    quickSwitcherOpen = false;
    void openNote(entry.relativePath);
  }

  function pickTag(tag: string): void {
    tagsViewOpen = false;
    onFilterChange(`tag:${tag}`);
  }

  // --- Wiki-links ----------------------------------------------------------
  async function findNoteByTitle(title: string): Promise<string | null> {
    // Recherche simple : la première note dont le titre correspond.
    // On utilise SearchNotes pour ne pas charger toute la liste côté Go.
    try {
      const results = await SearchNotes(`"${title.replace(/"/g, '')}"`, 5);
      const match = results.find((n) => n.title === title);
      return match?.relativePath ?? null;
    } catch {
      return null;
    }
  }

  async function onWikiNavigate(target: string): Promise<void> {
    if (!(await flushSave())) return;
    const path = await findNoteByTitle(target);
    if (path) {
      void openNote(path);
    } else {
      showToast('info', `Note introuvable : ${target}. Ctrl+clic pour créer.`);
    }
  }

  async function onWikiCreate(target: string): Promise<void> {
    if (!(await flushSave())) return;
    try {
      const note = await CreateNote(target, 'blank');
      showToast('info', `Note « ${target} » créée.`);
      invalidateFolders();
      await refresh();
      await openNote(note.relativePath);
    } catch (err) {
      showToast('error', `Échec : ${err}`);
    }
  }

  // --- Assets (paste/drop d'images) ---------------------------------------
  async function onAssetUpload(file: File): Promise<string | null> {
    try {
      const buffer = new Uint8Array(await file.arrayBuffer());
      // Wails transporte []byte sous forme de base64 côté JS.
      const rel = await SaveAsset(Array.from(buffer), file.name);
      showToast('info', `Image enregistrée : ${rel}`);
      return rel;
    } catch (err) {
      showToast('error', `Échec upload : ${err}`);
      return null;
    }
  }

  // Importe un fichier depuis son chemin absolu (utilisé pour les drops
  // depuis un explorateur de fichiers sur WebKit Linux : le navigateur
  // expose le fichier comme `file://` au lieu de donner un File direct).
  async function onAssetImportFromPath(absolutePath: string): Promise<string | null> {
    try {
      const rel = await ImportAssetFromFilePath(absolutePath);
      showToast('info', `Image importée : ${rel}`);
      return rel;
    } catch (err) {
      showToast('error', `Échec import : ${err}`);
      return null;
    }
  }

  // Transforme un chemin relatif d'asset (assets/2026/07/abc.png) en URL
  // HTTP absolue servie par le serveur interne de l'app. Cache les URLs
  // calculées pour éviter un round-trip Wails à chaque render d'image.
  const assetURLCache = new Map<string, string>();
  async function assetURL(relPath: string): Promise<string> {
    if (!isLocalAssetPath(relPath)) return relPath;
    const cached = assetURLCache.get(relPath);
    if (cached) return cached;
    try {
      const url = await AssetURL(relPath);
      assetURLCache.set(relPath, url);
      return url;
    } catch (err) {
      console.error('[assetURL] failed:', err);
      return relPath;
    }
  }

  // --- History -------------------------------------------------------------
  function openHistory(): void {
    if (!selected) return;
    historyOpen = true;
  }

  async function restoreFromHistory(versionID: string): Promise<void> {
    if (!selected) return;
    try {
      const restored = await RestoreFromHistory(selected.relativePath, versionID);
      const content = await precomputeAssetURLs(restored.content);
      selected = cloneNote(restored, content);
      lastSavedSnapshot = snapshot(selected!);
      saveState = 'clean';
      lastSavedAt = new Date();
      historyOpen = false;
      showToast('info', 'Version restaurée.');
      invalidateFolders();
      await refresh();
    } catch (err) {
      showToast('error', `Échec : ${err}`);
    }
  }

  // --- Vault picker --------------------------------------------------------
  function isSyncCandidate(p: string): boolean {
    const norm = p.toLowerCase();
    return (
      norm.includes('dropbox') ||
      norm.includes('icloud') ||
      norm.includes('syncthing') ||
      norm.includes('onedrive')
    );
  }

  $effect(() => {
    vaultIsSynced = isSyncCandidate(vaultPath);
  });

  function openVaultPicker(): void {
    vaultMenuOpen = false;
    vaultSwitchError = '';
    vaultPickerOpen = true;
  }

  async function prepareVaultSwitch(): Promise<boolean> {
    if (applicationStatus?.mode === 'noVault' || applicationStatus?.mode === 'locked') {
      clearSaveTimers();
      return true;
    }
    noteEditor?.flushPendingChange();
    const needsSave = hasUnsavedChanges || saveState === 'dirty' || saveState === 'saving';
    if (needsSave && !window.confirm('Enregistrer les modifications et changer de coffre ?')) {
      return false;
    }
    clearSaveTimers();
    if (needsSave && !(await flushSave())) {
      vaultSwitchError = 'La sauvegarde a échoué. Le coffre n’a pas été changé.';
      return false;
    }
    try {
      await ClearDirtyBuffer();
    } catch (err) {
      vaultSwitchError = `Impossible de vider le buffer de récupération : ${err}`;
      return false;
    }
    return true;
  }

  function resetVaultState(): void {
    refreshSeq += 1;
    clearSaveTimers();
    notes = [];
    chatNotes = [];
    pinned = [];
    folders = [];
    tags = [];
    templates = [];
    selected = null;
    lastSavedSnapshot = '';
    saveState = 'clean';
    lastSavedAt = null;
    activeFilter = '';
    activeChips = [];
    parsedFilter = null;
    allEntries = [];
    foldersLoaded = false;
    recoverySnapshot = null;
    recoveryOpen = false;
    onboardingOpen = false;
    quickSwitcherOpen = false;
    tagsViewOpen = false;
    templatePickerOpen = false;
    moveDialogOpen = false;
    historyOpen = false;
    statsOpen = false;
    exportOpen = false;
    chatOpen = false;
    encryptionDialogOpen = false;
  }

  async function finishVaultSwitch(): Promise<void> {
    resetVaultState();
    applicationStatus = await ApplicationStatus();
    vaultPath = applicationStatus.activeVault?.path ?? '';
    vaultPickerOpen = false;
    vaultMenuOpen = false;
    if (applicationStatus.mode === 'noVault') {
      vaultStatus = null;
      loading = false;
      return;
    }
    vaultStatus = await VaultStatus();
    if (applicationStatus.mode === 'locked') {
      loading = false;
      return;
    }
    await refresh();
    await checkStartup();
  }

  async function switchVault(path: string): Promise<void> {
    if (vaultSwitching || !(await prepareVaultSwitch())) return;
    vaultSwitching = true;
    vaultSwitchError = '';
    try {
      await OpenVault(path);
      await finishVaultSwitch();
    } catch (err) {
      vaultSwitchError = String(err);
    } finally {
      vaultSwitching = false;
    }
  }

  async function createVault(request: domain.CreateVaultRequest): Promise<void> {
    if (vaultSwitching || !(await prepareVaultSwitch())) return;
    vaultSwitching = true;
    vaultSwitchError = '';
    try {
      await CreateVault(request);
      await finishVaultSwitch();
    } catch (err) {
      vaultSwitchError = String(err);
    } finally {
      vaultSwitching = false;
    }
  }

  async function forgetRecentVault(path: string): Promise<void> {
    try {
      await ForgetRecentVault(path);
      applicationStatus = await ApplicationStatus();
    } catch (err) {
      vaultSwitchError = String(err);
    }
  }

  void bootstrapVault();

  async function bootstrapVault(): Promise<void> {
    try {
      applicationStatus = await ApplicationStatus();
      vaultPath = applicationStatus.activeVault?.path ?? '';
      if (applicationStatus.mode === 'noVault') {
        loading = false;
        return;
      }
      vaultStatus = await VaultStatus();
      if (applicationStatus.mode === 'locked') {
        loading = false;
        return;
      }
      await refresh();
      await checkStartup();
    } catch (err) {
      error = `Impossible d’initialiser le coffre : ${err}`;
      loading = false;
    }
  }

  async function unlockVault(): Promise<void> {
    if (unlocking) return;
    unlocking = true;
    unlockError = '';
    try {
      await UnlockVault(unlockPassphrase);
      unlockPassphrase = '';
      vaultStatus = await VaultStatus();
      applicationStatus = await ApplicationStatus();
      await refresh();
      await checkStartup();
      if (vaultStatus.warnings.length > 0) {
        showToast('error', 'Certaines notes illisibles ont été exclues de l’index.');
      }
    } catch {
      unlockError = 'Impossible de déverrouiller le coffre. Vérifiez la phrase secrète.';
      requestAnimationFrame(() => document.getElementById('vault-passphrase')?.focus());
    } finally {
      unlocking = false;
    }
  }

  function openEncryptionDialog(): void {
    encryptionAction = vaultStatus?.encryptionEnabled ? 'change' : 'enable';
    currentPassphrase = '';
    replacementPassphrase = '';
    confirmationPassphrase = '';
    encryptionError = '';
    encryptionDialogOpen = true;
  }

  async function submitEncryption(): Promise<void> {
    encryptionError = '';
    if (encryptionAction !== 'disable' && replacementPassphrase !== confirmationPassphrase) {
      encryptionError = 'Les deux phrases secrètes ne correspondent pas.';
      return;
    }
    encryptionBusy = true;
    try {
      if (encryptionAction === 'enable') {
        await EnableEncryption(replacementPassphrase);
      } else if (encryptionAction === 'change') {
        await ChangePassphrase(currentPassphrase, replacementPassphrase);
      } else {
        await DisableEncryption(currentPassphrase);
      }
      vaultStatus = await VaultStatus();
      encryptionDialogOpen = false;
      showToast('info', encryptionAction === 'disable' ? 'Chiffrement désactivé.' : 'Chiffrement mis à jour.');
    } catch (err) {
      encryptionError = `${err}`;
      vaultStatus = await VaultStatus();
      if (vaultStatus.state === 'locked') {
        encryptionDialogOpen = false;
        selected = null;
        notes = [];
        pinned = [];
      }
    } finally {
      currentPassphrase = '';
      replacementPassphrase = '';
      confirmationPassphrase = '';
      encryptionBusy = false;
    }
  }

  async function checkStartup(): Promise<void> {
    try {
      const snap = (await SnapshotForStartup()) as vault.RecoverySnapshot;
      if (!snap) return;
      recoverySnapshot = snap;
      if (snap.hasRecovery) {
        recoveryOpen = true;
        return;
      }
      maybeOpenGuide();
    } catch (err) {
      console.error('[startup] snapshot failed:', err);
    }
  }

  function maybeOpenGuide(): void {
    if (applicationStatus?.onboardingDismissed || onboardingShownThisLaunch) return;
    onboardingShownThisLaunch = true;
    onboardingOpen = true;
  }

  function reviewGuide(): void {
    shortcutsOpen = false;
    onboardingShownThisLaunch = true;
    onboardingOpen = true;
  }

  function onOnboardingDone(dismissed: boolean): void {
    onboardingOpen = false;
    if (applicationStatus) {
      applicationStatus.onboardingDismissed = dismissed;
      applicationStatus = applicationStatus;
    }
  }

  async function onRecoverAccept(buffer: string): Promise<void> {
    const recoveryPath = recoverySnapshot?.notePath;
    if (!recoveryPath) return;
    try {
      if (!selected || selected.relativePath !== recoveryPath) {
        const recoveredNote = await OpenNote(recoveryPath);
        selected = cloneNote(recoveredNote, await precomputeAssetURLs(recoveredNote.content));
      }
    } catch (err) {
      showToast('error', `Impossible d’ouvrir la note à récupérer : ${err}`);
      return;
    }
    selected!.content = buffer;
    selected = selected;
    saveState = 'dirty';
    lastSavedSnapshot = ''; // force la prochaine save à persister
    recoveryOpen = false;
    try {
      await ClearDirtyBuffer();
    } catch (err) {
      console.error('[recovery] clear failed:', err);
    }
    showToast('info', 'Buffer récupéré. Enregistrez pour conserver les modifications.');
    maybeOpenGuide();
  }

  async function onRecoverDiscard(): Promise<void> {
    recoveryOpen = false;
    try {
      await ClearDirtyBuffer();
    } catch (err) {
      console.error('[recovery] clear failed:', err);
    }
    showToast('info', 'Buffer ignoré.');
    maybeOpenGuide();
  }

  function onExportSuccess(path: string): void {
    showToast('info', `Archive créée : ${path}`);
  }

  function onStatsPickTag(tag: string): void {
    statsOpen = false;
    onFilterChange(`tag:${tag}`);
  }

  async function openChat(): Promise<void> {
    if (vaultStatus?.encryptionEnabled) {
      chatOpen = true;
      return;
    }
    if (!(await beforeChatPrepare())) return;
    chatOpen = true;
  }

  async function beforeChatPrepare(): Promise<boolean> {
    noteEditor?.flushPendingChange();
    if (!selected || (!hasUnsavedChanges && saveState !== 'dirty' && saveState !== 'saving')) {
      return true;
    }
    return flushSave();
  }
</script>

<svelte:window onkeydown={onGlobalKeydown} onbeforeunload={onBeforeUnload} />

<div class="grid h-full min-h-0 grid-rows-[2.25rem_minmax(0,1fr)] bg-background text-foreground">
  <WindowTitleBar onClose={onWindowClose} />

  {#if applicationStatus === null}
    <main class="grid min-h-0 place-items-center" aria-live="polite">
      <p class="text-sm text-subtle">Chargement de NoteVault…</p>
    </main>
  {:else if applicationStatus.mode === 'noVault'}
    <main class="flex min-h-0" aria-label="Sélection du coffre">
      <VaultPickerDialog
        open={true}
        embedded={true}
        status={applicationStatus}
        busy={vaultSwitching}
        error={vaultSwitchError}
        onOpen={(path) => void switchVault(path)}
        onCreate={(request) => void createVault(request)}
        onForget={(path) => void forgetRecentVault(path)}
        onClose={() => {}}
      />
    </main>
  {:else if shouldShowVaultUnlock(applicationStatus.mode, vaultStatus?.state)}
    <main class="grid min-h-0 place-items-center px-6" aria-labelledby="unlock-title">
      <form class="w-full max-w-sm" onsubmit={(event) => { event.preventDefault(); void unlockVault(); }}>
        <h1 id="unlock-title" class="text-lg font-semibold">Coffre verrouillé</h1>
        <p class="mt-2 text-sm leading-6 text-subtle">
          Saisissez votre phrase secrète pour charger les notes et reconstruire l’index local.
        </p>
        <label for="vault-passphrase" class="mt-6 block text-sm font-medium">Phrase secrète</label>
        <input
          id="vault-passphrase"
          bind:this={unlockInput}
          class="mt-2 h-10 w-full rounded-md border border-border-strong bg-panel px-3 text-foreground"
          type="password"
          autocomplete="current-password"
          bind:value={unlockPassphrase}
          aria-describedby="unlock-feedback"
          disabled={unlocking}
        />
        <div id="unlock-feedback" class="mt-2 min-h-6 text-sm text-danger" role="status" aria-live="polite">
          {unlocking ? 'Déverrouillage et reconstruction de l’index en cours…' : unlockError}
        </div>
        <button
          class="mt-3 h-10 w-full rounded-md border border-accent bg-accent px-4 text-sm font-medium text-accent-foreground hover:bg-accent-hover"
          type="submit"
          disabled={unlocking || unlockPassphrase.length === 0}
        >
          {unlocking ? 'Déverrouillage et indexation…' : 'Déverrouiller'}
        </button>
        <button class="mt-2 h-9 w-full rounded-md border border-border bg-panel px-3 text-sm text-subtle hover:bg-panel-muted hover:text-foreground" type="button" onclick={openVaultPicker} disabled={unlocking}>
          Choisir un autre coffre
        </button>
        {#if (vaultStatus?.warnings?.length ?? 0) > 0}
          <div class="mt-5 border-l-2 border-danger pl-3 text-sm text-subtle" role="status">
            Certains fichiers ont demandé votre attention lors de la dernière ouverture.
          </div>
        {/if}
      </form>
    </main>
  {:else}
  <div class={sidebarCollapsed
    ? 'grid h-full min-h-0 grid-rows-[3rem_minmax(0,1fr)] lg:grid-cols-[3.25rem_minmax(0,1fr)] lg:grid-rows-none'
    : 'grid h-full min-h-0 grid-rows-[14rem_minmax(0,1fr)] lg:grid-cols-[20rem_minmax(0,1fr)] lg:grid-rows-none'}>
    <aside
      bind:this={sidebarEl}
      class="flex min-h-0 flex-col border-b border-border bg-sidebar lg:border-b-0 lg:border-r"
      onfocusin={onSidebarFocus}
      aria-label="Navigation des notes"
    >
    {#if sidebarCollapsed}
    <div class="flex min-h-0 flex-1 items-center gap-1.5 px-2 py-1.5 lg:flex-col lg:px-0 lg:py-2">
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-panel hover:text-foreground"
        type="button"
        title="Déplier la barre latérale (Ctrl+Shift+B)"
        aria-label="Déplier la barre latérale"
        aria-expanded="false"
        onclick={toggleSidebar}
      >
        <PanelLeftOpen size={15} strokeWidth={2} aria-hidden="true" />
      </button>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-panel hover:text-foreground"
        type="button"
        title="Recherche rapide (Ctrl+P)"
        aria-label="Recherche rapide"
        onclick={() => (quickSwitcherOpen = true)}
      >
        <Search size={15} strokeWidth={2} aria-hidden="true" />
      </button>
      <button
        class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-panel hover:text-foreground"
        type="button"
        title="Nouvelle note (Ctrl+N)"
        aria-label="Nouvelle note"
        onclick={() => openTemplatePicker()}
      >
        <Plus size={15} strokeWidth={2} aria-hidden="true" />
      </button>
    </div>
    {:else}
    <div class="relative flex h-14 shrink-0 items-center justify-between gap-2 border-b border-border px-3">
      <div class="min-w-0 flex-1">
        <button
          type="button"
          class="flex w-full min-w-0 items-center gap-2 rounded-md px-1 py-1 text-left hover:bg-panel-muted"
          onclick={() => (vaultMenuOpen = !vaultMenuOpen)}
          title={vaultPath}
          aria-haspopup="menu"
          aria-expanded={vaultMenuOpen}
        >
          <span class="min-w-0 flex-1">
            <span class="flex items-center gap-1.5">
              <strong class="truncate text-sm font-semibold tracking-normal">{applicationStatus.activeVault?.name ?? 'Coffre'}</strong>
              {#if vaultIsSynced}<Cloud size={12} strokeWidth={2} class="shrink-0 text-accent" aria-label="Coffre dans un dossier synchronisé" />{/if}
            </span>
            <span class="block truncate text-xs text-subtle">{vaultPath}</span>
          </span>
          <ChevronDown size={14} class="shrink-0 text-subtle" aria-hidden="true" />
        </button>
        {#if vaultMenuOpen}
          <div class="absolute left-2 top-[3.2rem] z-40 w-[min(19rem,calc(100vw-1rem))] overflow-hidden rounded-md border border-border bg-panel shadow-lg" role="menu" aria-label="Changer de coffre">
            {#each applicationStatus.recentVaults as recent (recent.path)}
              <button type="button" role="menuitem" class="block w-full border-b border-border px-3 py-2 text-left hover:bg-panel-muted disabled:text-faint" disabled={vaultSwitching || !recent.available || recent.active} onclick={() => void switchVault(recent.path)}>
                <span class="flex items-center gap-2 text-sm"><span class="truncate">{recent.name}</span>{#if recent.encrypted}<span class="text-xs text-subtle">Chiffré</span>{/if}{#if !recent.available}<span class="text-xs text-danger">Indisponible</span>{/if}</span>
                <span class="block truncate text-xs text-subtle">{recent.path}</span>
              </button>
            {/each}
            <button type="button" role="menuitem" class="block w-full px-3 py-2 text-left text-sm hover:bg-panel-muted" onclick={openVaultPicker}>Créer ou ouvrir un coffre…</button>
          </div>
        {/if}
      </div>
      <div class="ml-2 flex items-center gap-1.5">
        <button
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          title="Replier la barre latérale (Ctrl+Shift+B)"
          aria-label="Replier la barre latérale"
          aria-expanded="true"
          onclick={toggleSidebar}
        >
          <PanelLeftClose size={13} strokeWidth={2} aria-hidden="true" />
        </button>
        <button
          class="rounded-md border border-border-strong px-2 py-0.5 text-xs text-subtle hover:text-foreground"
          type="button"
          onclick={openEncryptionDialog}
          aria-label="Configurer le chiffrement"
        >
          {vaultStatus?.encryptionEnabled ? 'Chiffré' : 'Sécurité'}
        </button>
        <span class="rounded-md border border-border-strong px-2 py-0.5 text-xs text-subtle">
          {notes.length}
        </span>
      </div>
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
              ? 'inline-flex h-7 w-7 items-center justify-center rounded text-foreground'
              : 'inline-flex h-7 w-7 items-center justify-center rounded text-subtle hover:text-foreground'}
            type="button"
            title="Vue à plat"
            aria-label="Vue à plat"
            aria-pressed={view === 'flat'}
            onclick={() => setView('flat')}
          >
            <LayoutList size={13} strokeWidth={2} aria-hidden="true" />
          </button>
          <button
            class={view === 'tree'
              ? 'inline-flex h-7 w-7 items-center justify-center rounded text-foreground'
              : 'inline-flex h-7 w-7 items-center justify-center rounded text-subtle hover:text-foreground'}
            type="button"
            title="Vue arborescente"
            aria-label="Vue arborescente"
            aria-pressed={view === 'tree'}
            onclick={() => setView('tree')}
          >
            <FolderTree size={13} strokeWidth={2} aria-hidden="true" />
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
            title="Tags (Ctrl+T)"
            aria-label="Vue Tags"
            onclick={() => (tagsViewOpen = true)}
          >
            <Hash size={13} strokeWidth={2} aria-hidden="true" />
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
            onclick={() => openTemplatePicker()}
          >
            <Plus size={13} strokeWidth={2} aria-hidden="true" />
          </button>
        </div>
      </div>

      {#if loading}
        <div class="flex flex-col gap-2" aria-busy="true">
          {#each [0, 1, 2, 3, 4] as row (row)}
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
                <div
                  class={active
                    ? 'flex w-full items-center gap-1.5 rounded-md border border-accent bg-accent/15 px-2 py-1 text-foreground'
                    : 'flex w-full items-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-foreground hover:border-border hover:bg-panel-muted'}
                  role="button"
                  tabindex="0"
                  aria-current={active ? 'page' : undefined}
                  draggable="true"
                  ondragstart={(e) => onDragStart(e, note.relativePath)}
                  ondragend={onDragEnd}
                  onclick={() => openNote(note.relativePath)}
                  oncontextmenu={(e) => openContextMenu(e, note.relativePath)}
                  onkeydown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      void openNote(note.relativePath);
                    }
                  }}
                >
                  <GripVertical size={10} strokeWidth={2} class="shrink-0 text-faint" aria-hidden="true" />
                  <Pin size={11} strokeWidth={2.5} class="shrink-0 text-accent" aria-hidden="true" />
                  <span class="min-w-0 flex-1 truncate text-sm">{note.title || 'Sans titre'}</span>
                </div>
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
              selectedPath={selectedPath}
              onOpen={openNote}
              onDragStart={onDragStart}
              onDragEnd={onDragEnd}
              onFolderDragOver={onFolderDragOver}
              onFolderDragLeave={onFolderDragLeave}
              onFolderDrop={onFolderDrop}
              onContextMenu={openContextMenu}
              dragOverFolder={dragOverFolder}
              onTogglePin={(p) => {
                if (selected?.relativePath === p) {
                  void togglePinCurrent();
                } else {
                  void PinNote(p, !pinnedSet.has(p)).then(() => refreshPinnedAndTags());
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
                <div
                  class={active
                    ? 'grid w-full cursor-grab gap-1 rounded-lg border border-accent bg-panel px-3 py-2 text-left text-foreground shadow-sm'
                    : 'grid w-full cursor-grab gap-1 rounded-lg border border-transparent px-3 py-2 text-left text-foreground hover:border-border hover:bg-panel-muted'}
                  role="button"
                  tabindex="0"
                  aria-current={active ? 'page' : undefined}
                  draggable="true"
                  ondragstart={(e) => onDragStart(e, note.relativePath)}
                  ondragend={onDragEnd}
                  onclick={() => openNote(note.relativePath)}
                  oncontextmenu={(e) => openContextMenu(e, note.relativePath)}
                  onkeydown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault();
                      void openNote(note.relativePath);
                    }
                  }}
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
                </div>
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
    {/if}
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
          class={chatOpen
            ? 'inline-flex h-9 items-center gap-1.5 rounded-md border border-accent bg-accent/15 px-2.5 text-xs font-medium text-accent'
            : 'inline-flex h-9 items-center gap-1.5 rounded-md border border-border bg-panel-muted px-2.5 text-xs text-subtle hover:bg-sidebar hover:text-foreground'}
          type="button"
          title="Discuter avec les notes (Ctrl+Shift+C)"
          aria-label="Discuter avec les notes"
          aria-pressed={chatOpen}
          onclick={() => (chatOpen ? (chatOpen = false) : void openChat())}
        >
          <MessageSquare size={14} strokeWidth={2} aria-hidden="true" />
          <span class="hidden 2xl:inline">Chat</span>
        </button>
        <button
          class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
          type="button"
          title="Activité (Ctrl+Shift+G)"
          aria-label="Activité"
          onclick={() => (statsOpen = true)}
        >
          <Activity size={15} strokeWidth={2} aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
          type="button"
          title="Exporter (Ctrl+Shift+E)"
          aria-label="Exporter"
          onclick={() => (exportOpen = true)}
        >
          <Download size={15} strokeWidth={2} aria-hidden="true" />
        </button>
        <button
          class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-panel-muted text-subtle hover:bg-sidebar hover:text-foreground"
          type="button"
          title="Raccourcis (Ctrl+/)"
          aria-label="Raccourcis"
          onclick={() => (shortcutsOpen = true)}
        >
          <Keyboard size={15} strokeWidth={2} aria-hidden="true" />
        </button>
        <ThemeMenu active={activeThemeId} onSelect={(id) => void selectTheme(id)} />
      </div>
    </header>

    <div class={chatOpen
      ? 'grid h-full min-h-0 overflow-hidden xl:grid-cols-[minmax(0,1fr)_30rem]'
      : 'grid h-full min-h-0 overflow-hidden'}>
    <section class="flex min-h-0 min-w-0 flex-col overflow-hidden" aria-label="Éditeur de note">
      {#if error}
        <p class="mx-4 mt-4 rounded-lg border border-danger/40 bg-panel px-3 py-2 text-sm text-danger" role="alert">
          {error}
        </p>
      {/if}

      {#if selected}
        <div class="flex min-h-0 flex-1 flex-col">
          {#if titleEditing}
            <input
              bind:this={titleEl}
              class="block w-full shrink-0 border-0 bg-transparent px-4 pb-3 pt-5 text-3xl font-semibold leading-tight text-foreground outline-none placeholder:text-faint focus:outline-none focus-visible:outline-none sm:text-4xl"
              aria-label="Titre de la note"
              bind:value={titleDraft}
              onkeydown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault();
                  void commitRename();
                } else if (e.key === 'Escape') {
                  e.preventDefault();
                  cancelRename();
                }
              }}
              onblur={() => commitRename()}
              placeholder="Sans titre"
            />
          {:else}
            <h1
              class="block w-full shrink-0 cursor-text select-none border-0 bg-transparent px-4 pb-2 pt-5 text-3xl font-semibold leading-tight text-foreground sm:text-4xl"
              ondblclick={startRename}
              title="Double-cliquer pour renommer (Ctrl+Shift+R)"
            >
              {selected.title || 'Sans titre'}
            </h1>
          {/if}

          <div class="flex shrink-0 flex-wrap items-center gap-3 border-b border-border/60 px-4 py-2">
            <TagEditor
              tags={selected.tags ?? []}
              knownTags={tags}
              onChange={onTagsChange}
            />
            {#if selected.relativePath.includes('/')}
              <span class="text-xs text-faint">
                Dossier : {selected.relativePath.split('/').slice(1, -1).join('/') || 'racine'}
              </span>
            {/if}
          </div>

          <div class="min-h-0 flex-1">
            {#key selected.relativePath}
              <NoteEditor
                bind:this={noteEditor}
                markdown={selected.content}
                onChange={onEditorChange}
                onDirty={onEditorDirty}
                knownTitles={knownTitles}
                onWikiNavigate={onWikiNavigate}
                onWikiCreate={onWikiCreate}
                onAssetUpload={onAssetUpload}
                onAssetImportFromPath={onAssetImportFromPath}
                assetURL={assetURL}
              />
            {/key}
          </div>

          <BacklinksPanel
            title={selected.title}
            excludePath={selected.relativePath}
            onOpen={openNote}
          />

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
                  class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-transparent px-2.5 text-sm font-medium text-subtle hover:bg-panel-muted hover:text-foreground"
                  type="button"
                  onclick={openHistory}
                  title="Historique (Ctrl+Shift+H)"
                  disabled={saving || deleting}
                  aria-label="Historique"
                >
                  <History size={14} strokeWidth={2} aria-hidden="true" />
                </button>
                <button
                  class="inline-flex items-center gap-2 rounded-md border border-border bg-transparent px-3 py-1.5 text-sm font-medium text-subtle hover:bg-panel-muted hover:text-foreground"
                  type="button"
                  onclick={openMoveDialog}
                  title="Déplacer (Ctrl+Shift+M)"
                  disabled={saving || deleting}
                >
                  <FolderInput size={14} strokeWidth={2} aria-hidden="true" />
                  Déplacer
                </button>
                <button
                  class="inline-flex items-center gap-2 rounded-md border border-danger/45 bg-transparent px-3 py-1.5 text-sm font-medium text-danger hover:bg-danger/10 disabled:hover:bg-transparent"
                  type="button"
                  onclick={requestDelete}
                  disabled={deleting || saving}
                >
                  <Trash2 size={14} strokeWidth={2} aria-hidden="true" />
                  Supprimer
                </button>
                <button
                  class="inline-flex items-center gap-2 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover disabled:hover:bg-accent"
                  type="button"
                  onclick={saveSelected}
                  disabled={saving || deleting}
                >
                  <Save size={14} strokeWidth={2} aria-hidden="true" />
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
              recherche ·
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+T</kbd>
              tags ·
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+N</kbd>
              nouvelle note ·
              <kbd class="rounded border border-border-strong bg-background px-1.5 py-0.5">Ctrl+Shift+R</kbd>
              renommer
            </p>
          </div>
        </div>
      {/if}
    </section>
    {#key vaultPath}
      <ChatPanel
        open={chatOpen}
        notes={chatNotes}
        availableTags={tags}
        currentPath={selected?.relativePath ?? ''}
        encrypted={vaultStatus?.encryptionEnabled ?? false}
        beforePrepare={beforeChatPrepare}
        onOpenNote={(path) => void openNote(path)}
        onClose={() => (chatOpen = false)}
      />
    {/key}
    </div>
    </main>
  </div>
  {/if}
</div>

{#if encryptionDialogOpen}
  <div class="fixed inset-0 z-50 grid place-items-center px-4">
    <button
      class="absolute inset-0 bg-black/55"
      type="button"
      aria-label="Fermer les réglages de chiffrement"
      onclick={() => (encryptionDialogOpen = false)}
      disabled={encryptionBusy}
    ></button>
    <div
      class="relative w-full max-w-md rounded-lg border border-border bg-panel p-5 shadow-lg"
      role="dialog"
      aria-modal="true"
      aria-labelledby="encryption-title"
    >
    <form
      aria-busy={encryptionBusy}
      onsubmit={(event) => { event.preventDefault(); void submitEncryption(); }}
    >
      <h2 id="encryption-title" class="text-base font-semibold">Chiffrement du coffre</h2>
      <p class="mt-2 text-sm leading-6 text-subtle">
        Une phrase oubliée rend les notes irrécupérables. Les noms de fichiers, dossiers, épingles et assets restent visibles.
      </p>
      {#if !vaultStatus?.encryptionEnabled}
        <p class="mt-2 text-xs leading-5 text-subtle">
          L’ancien index est supprimé à l’activation, sans garantie d’effacement sur un SSD, un snapshot ou une sauvegarde. Les fichiers <code>.md</code> ne seront plus lisibles par un éditeur externe.
        </p>
      {/if}

      {#if vaultStatus?.encryptionEnabled}
        <div class="mt-5 flex gap-4 border-b border-border pb-2" role="group" aria-label="Action de chiffrement">
          <button aria-pressed={encryptionAction === 'change'} class="text-sm {encryptionAction === 'change' ? 'text-foreground' : 'text-subtle'}" type="button" onclick={() => (encryptionAction = 'change')}>Changer la phrase</button>
          <button aria-pressed={encryptionAction === 'disable'} class="text-sm {encryptionAction === 'disable' ? 'text-danger' : 'text-subtle'}" type="button" onclick={() => (encryptionAction = 'disable')}>Désactiver</button>
        </div>
        <label for="current-passphrase" class="mt-5 block text-sm font-medium">Phrase secrète actuelle</label>
        <input id="current-passphrase" class="mt-2 h-10 w-full rounded-md border border-border-strong bg-background px-3" type="password" autocomplete="current-password" bind:value={currentPassphrase} disabled={encryptionBusy} />
      {/if}

      {#if encryptionAction !== 'disable'}
        <label for="new-passphrase" class="mt-5 block text-sm font-medium">Nouvelle phrase secrète</label>
        <input id="new-passphrase" class="mt-2 h-10 w-full rounded-md border border-border-strong bg-background px-3" type="password" autocomplete="new-password" minlength="12" maxlength="1024" bind:value={replacementPassphrase} disabled={encryptionBusy} />
        <label for="confirm-passphrase" class="mt-4 block text-sm font-medium">Confirmer la phrase secrète</label>
        <input id="confirm-passphrase" class="mt-2 h-10 w-full rounded-md border border-border-strong bg-background px-3" type="password" autocomplete="new-password" minlength="12" maxlength="1024" bind:value={confirmationPassphrase} disabled={encryptionBusy} />
      {/if}

      <div class="mt-3 min-h-6 text-sm text-danger" role="status" aria-live="polite">{encryptionBusy ? 'Conversion des fichiers en cours…' : encryptionError}</div>
      <div class="mt-4 flex justify-end gap-2">
        <button class="rounded-md border border-border px-3 py-2 text-sm text-subtle" type="button" onclick={() => (encryptionDialogOpen = false)} disabled={encryptionBusy}>Annuler</button>
        <button class="rounded-md border px-3 py-2 text-sm font-medium {encryptionAction === 'disable' ? 'border-danger bg-danger text-background' : 'border-accent bg-accent text-accent-foreground'}" type="submit" disabled={encryptionBusy}>
          {encryptionBusy ? 'Traitement…' : encryptionAction === 'enable' ? 'Activer' : encryptionAction === 'change' ? 'Changer' : 'Désactiver'}
        </button>
      </div>
    </form>
    </div>
  </div>
{/if}

{#if vaultSwitching}
  <div class="fixed inset-0 z-[70] grid place-items-center bg-black/45 px-4" role="status" aria-live="assertive" aria-label="Changement de coffre en cours">
    <div class="rounded-md border border-border bg-panel px-4 py-3 text-sm text-foreground shadow-lg">Préparation et indexation du coffre…</div>
  </div>
{/if}

<QuickSwitcher
  open={quickSwitcherOpen}
  entries={allEntries}
  onPick={pickEntry}
  onClose={() => (quickSwitcherOpen = false)}
/>

<TagsView
  open={tagsViewOpen}
  tags={tags}
  onPick={pickTag}
  onClose={() => (tagsViewOpen = false)}
/>

<TemplatePickerDialog
  open={templatePickerOpen}
  templates={templates}
  onPick={createNoteFromTemplate}
  onClose={() => (templatePickerOpen = false)}
/>

<MoveDialog
  open={moveDialogOpen}
  currentPath={moveTarget}
  folders={folders}
  onMove={moveTo}
  onClose={() => (moveDialogOpen = false)}
/>

<HistoryPanel
  open={historyOpen}
  relativePath={selected?.relativePath ?? ''}
  onRestore={restoreFromHistory}
  onClose={() => (historyOpen = false)}
/>

<VaultPickerDialog
  open={vaultPickerOpen}
  status={applicationStatus}
  busy={vaultSwitching}
  error={vaultSwitchError}
  onOpen={(path) => void switchVault(path)}
  onCreate={(request) => void createVault(request)}
  onForget={(path) => void forgetRecentVault(path)}
  onClose={() => (vaultPickerOpen = false)}
/>

<OnboardingModal
  open={onboardingOpen}
  initialTheme={theme}
  initiallyDismissed={applicationStatus?.onboardingDismissed ?? false}
  onDone={onOnboardingDone}
/>

<ShortcutsOverlay open={shortcutsOpen} onClose={() => (shortcutsOpen = false)} onReviewGuide={reviewGuide} />

<StatsView
  open={statsOpen}
  onClose={() => (statsOpen = false)}
  onPickTag={onStatsPickTag}
/>

<ExportDialog
  open={exportOpen}
  notes={notes}
  encrypted={vaultStatus?.encryptionEnabled ?? false}
  defaultFilename={`notevault-${new Date().toISOString().slice(0, 10)}.zip`}
  onClose={() => (exportOpen = false)}
  onSuccess={onExportSuccess}
/>

<RecoveryDialog
  open={recoveryOpen}
  snapshot={recoverySnapshot}
  onRecover={onRecoverAccept}
  onDiscard={onRecoverDiscard}
  onClose={() => (recoveryOpen = false)}
/>

{#if contextMenu}
  <ContextMenu
    open={true}
    x={contextMenu.x}
    y={contextMenu.y}
    items={contextMenu.items}
    onClose={() => (contextMenu = null)}
  />
{/if}

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
