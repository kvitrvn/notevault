<script lang="ts">
  import Download from '@lucide/svelte/icons/download';
  import FileArchive from '@lucide/svelte/icons/file-archive';
  import FileText from '@lucide/svelte/icons/file-text';
  import X from '@lucide/svelte/icons/x';
  import {
    ExportNotePDF,
    ExportNotes,
    PDFExportOptions
  } from '../../wailsjs/go/main/App';
  import type { domain, vault } from '../../wailsjs/go/models';
  import { canCloseExportDialog, pdfExportBlocker } from '../lib/pdf-export';

  type NoteSummary = domain.NoteSummary;
  type ExportTab = 'zip' | 'pdf';

  type Props = {
    open: boolean;
    notes: NoteSummary[];
    activeNote?: NoteSummary | null;
    defaultFilename: string;
    encrypted?: boolean;
    onBeforePDFExport?: () => Promise<boolean>;
    onClose: () => void;
    onSuccess: (path: string) => void;
  };

  let {
    open,
    notes,
    activeNote = null,
    defaultFilename,
    encrypted = false,
    onBeforePDFExport = async () => true,
    onClose,
    onSuccess
  }: Props = $props();

  let tab = $state<ExportTab>('zip');
  let selected = $state<Set<string>>(new Set());
  let filename = $state('');
  let busy = $state(false);
  let error = $state('');
  let zipPlaintextConfirmed = $state(false);
  let pdfPlaintextConfirmed = $state(false);
  let pdfOptions = $state<vault.PDFExportOptionsInfo | null>(null);
  let pdfOptionsLoading = $state(false);
  let pdfThemeID = $state('classic');
  let loadSequence = 0;
  let zipTabButton: HTMLButtonElement | undefined = $state();

  $effect(() => {
    if (open) {
      tab = 'zip';
      selected = new Set();
      filename = defaultFilename;
      error = '';
      zipPlaintextConfirmed = false;
      pdfPlaintextConfirmed = false;
      pdfThemeID = 'classic';
      void loadPDFOptions();
      requestAnimationFrame(() => zipTabButton?.focus());
    } else {
      loadSequence++;
    }
  });

  async function loadPDFOptions(): Promise<void> {
    const sequence = ++loadSequence;
    pdfOptionsLoading = true;
    pdfOptions = null;
    try {
      const loaded = await PDFExportOptions();
      if (sequence !== loadSequence || !open) return;
      pdfOptions = loaded;
      if (!loaded.themes.some((theme) => theme.id === pdfThemeID)) {
        pdfThemeID = loaded.themes[0]?.id ?? 'classic';
      }
    } catch (err) {
      if (sequence === loadSequence && open) {
        error = String(err);
      }
    } finally {
      if (sequence === loadSequence) {
        pdfOptionsLoading = false;
      }
    }
  }

  function chooseTab(next: ExportTab): void {
    if (busy) return;
    tab = next;
    error = '';
  }

  function onTabKey(event: KeyboardEvent): void {
    let next: ExportTab | null = null;
    if (event.key === 'ArrowLeft' || event.key === 'Home') next = 'zip';
    if (event.key === 'ArrowRight' || event.key === 'End') next = 'pdf';
    if (!next) return;
    event.preventDefault();
    chooseTab(next);
    requestAnimationFrame(() => document.getElementById(`export-tab-${next}`)?.focus());
  }

  function close(): void {
    if (canCloseExportDialog(busy)) onClose();
  }

  function toggle(rel: string): void {
    if (selected.has(rel)) selected.delete(rel);
    else selected.add(rel);
    selected = new Set(selected);
  }

  function selectAll(): void {
    selected = new Set(notes.map((note) => note.relativePath));
  }

  function clearAll(): void {
    selected = new Set();
  }

  async function commitZip(): Promise<void> {
    if (selected.size === 0) {
      error = 'Sélectionnez au moins une note.';
      return;
    }
    if (encrypted && !zipPlaintextConfirmed) {
      error = 'Confirmez que l’archive contiendra les notes en clair.';
      return;
    }
    const name = filename.trim() || defaultFilename;
    const safe = name.endsWith('.zip') ? name : name + '.zip';
    busy = true;
    error = '';
    try {
      await ExportNotes(Array.from(selected), safe);
      onSuccess(safe);
      onClose();
    } catch (err) {
      error = String(err);
    } finally {
      busy = false;
    }
  }

  async function commitPDF(): Promise<void> {
    const blocker = pdfExportBlocker({
      options: pdfOptions,
      activeNotePath: activeNote?.relativePath ?? '',
      encrypted,
      plaintextConfirmed: pdfPlaintextConfirmed
    });
    if (blocker) {
      error = blocker;
      return;
    }
    busy = true;
    error = '';
    try {
      if (!(await onBeforePDFExport())) {
        error = 'La note n’a pas pu être enregistrée avant l’export.';
        return;
      }
      const destination = await ExportNotePDF(
        activeNote!.relativePath,
        pdfThemeID,
        pdfPlaintextConfirmed
      );
      if (!destination) return;
      onSuccess(destination);
      onClose();
    } catch (err) {
      error = String(err);
    } finally {
      busy = false;
    }
  }

  function commit(): Promise<void> {
    return tab === 'pdf' ? commitPDF() : commitZip();
  }

  function onKey(event: KeyboardEvent): void {
    if (!open) return;
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
    } else if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
      event.preventDefault();
      if (!busy) void commit();
    }
  }
</script>

<svelte:window onkeydown={onKey} />

{#if open}
  <div
    class="fixed inset-0 z-50 grid place-items-start px-4 py-10"
    role="dialog"
    aria-modal="true"
    aria-labelledby="export-title"
    aria-busy={busy}
  >
    <button
      class="fixed inset-0 bg-black/55"
      type="button"
      aria-label="Fermer"
      onclick={close}
      disabled={busy}
    ></button>
    <div
      class="relative mx-auto flex max-h-[80vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-border bg-panel shadow-2xl"
    >
      <header class="flex items-center justify-between gap-2 border-b border-border bg-background px-4 py-3">
        <h2 id="export-title" class="flex items-center gap-1.5 text-sm font-semibold text-foreground">
          <Download size={15} strokeWidth={2} class="text-accent" aria-hidden="true" />
          Exporter
        </h2>
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-border bg-panel text-subtle hover:bg-panel-muted hover:text-foreground disabled:opacity-50"
          aria-label="Fermer"
          onclick={close}
          disabled={busy}
        >
          <X size={13} strokeWidth={2} aria-hidden="true" />
        </button>
      </header>

      <div class="border-b border-border bg-background px-4 pt-2" role="tablist" aria-label="Format d’export">
        <button
          id="export-tab-zip"
          bind:this={zipTabButton}
          type="button"
          role="tab"
          aria-selected={tab === 'zip'}
          aria-controls="export-panel-zip"
          tabindex={tab === 'zip' ? 0 : -1}
          class={tab === 'zip'
            ? 'border-b-2 border-accent px-3 py-2 text-sm font-medium text-foreground'
            : 'border-b-2 border-transparent px-3 py-2 text-sm text-subtle hover:text-foreground'}
          onclick={() => chooseTab('zip')}
          onkeydown={onTabKey}
          disabled={busy}
        >
          ZIP
        </button>
        <button
          id="export-tab-pdf"
          type="button"
          role="tab"
          aria-selected={tab === 'pdf'}
          aria-controls="export-panel-pdf"
          tabindex={tab === 'pdf' ? 0 : -1}
          class={tab === 'pdf'
            ? 'border-b-2 border-accent px-3 py-2 text-sm font-medium text-foreground'
            : 'border-b-2 border-transparent px-3 py-2 text-sm text-subtle hover:text-foreground'}
          onclick={() => chooseTab('pdf')}
          onkeydown={onTabKey}
          disabled={busy}
        >
          PDF
        </button>
      </div>

      {#if tab === 'zip'}
        <div
          id="export-panel-zip"
          role="tabpanel"
          aria-labelledby="export-tab-zip"
          class="flex min-h-0 flex-1 flex-col gap-3 px-4 py-3"
        >
          <p class="flex items-start gap-2 text-xs text-subtle">
            <FileArchive size={15} class="mt-0.5 shrink-0" aria-hidden="true" />
            <span>
              Sélectionnez les notes à inclure. Les images référencées
              (<code class="rounded bg-background px-1 py-0.5">assets/...</code>)
              sont ajoutées automatiquement.
            </span>
          </p>

          <div class="flex items-center gap-2">
            <button
              type="button"
              class="rounded-md border border-border bg-background px-2.5 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
              onclick={selectAll}
              disabled={notes.length === 0 || busy}
            >
              Tout
            </button>
            <button
              type="button"
              class="rounded-md border border-border bg-background px-2.5 py-1 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
              onclick={clearAll}
              disabled={selected.size === 0 || busy}
            >
              Aucun
            </button>
            <span class="ml-auto text-xs text-faint">{selected.size} / {notes.length}</span>
          </div>

          <ul
            class="min-h-0 flex-1 overflow-y-auto rounded-md border border-border bg-background"
            aria-label="Notes à exporter"
          >
            {#each notes as note (note.relativePath)}
              {@const checked = selected.has(note.relativePath)}
              <li>
                <label
                  class="flex cursor-pointer items-center gap-2 border-b border-border/50 px-3 py-1.5 text-sm last:border-b-0 hover:bg-panel-muted"
                >
                  <input
                    type="checkbox"
                    class="h-3.5 w-3.5 accent-accent"
                    checked={checked}
                    onchange={() => toggle(note.relativePath)}
                    disabled={busy}
                  />
                  <span class="min-w-0 flex-1 truncate">
                    <span class="text-foreground">{note.title || 'Sans titre'}</span>
                    <span class="ml-2 text-[0.7rem] text-faint">{note.relativePath}</span>
                  </span>
                </label>
              </li>
            {/each}
            {#if notes.length === 0}
              <li class="px-3 py-4 text-center text-sm text-subtle">Aucune note à exporter.</li>
            {/if}
          </ul>

          <label class="flex flex-col gap-1">
            <span class="text-xs font-medium text-subtle">Nom du fichier (écrit à la racine du coffre)</span>
            <input
              type="text"
              bind:value={filename}
              class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
              spellcheck="false"
              autocomplete="off"
              placeholder={defaultFilename}
              disabled={busy}
            />
          </label>

          {#if encrypted}
            <label class="flex items-start gap-2 border-l-2 border-danger px-3 py-2 text-xs leading-5 text-subtle">
              <input
                class="mt-1 h-3.5 w-3.5 accent-accent"
                type="checkbox"
                bind:checked={zipPlaintextConfirmed}
                disabled={busy}
              />
              <span>Je confirme que cette archive contiendra du Markdown en clair et ne sera plus protégée par le chiffrement du coffre.</span>
            </label>
          {/if}
        </div>
      {:else}
        <div
          id="export-panel-pdf"
          role="tabpanel"
          aria-labelledby="export-tab-pdf"
          class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 py-4"
        >
          <div class="flex items-start gap-3 rounded-md border border-border bg-background px-3 py-3">
            <FileText size={18} class="mt-0.5 shrink-0 text-accent" aria-hidden="true" />
            <div class="min-w-0">
              <p class="text-xs font-medium text-subtle">Note active</p>
              {#if activeNote}
                <p class="truncate text-sm text-foreground">{activeNote.title || 'Sans titre'}</p>
                <p class="truncate text-xs text-faint">{activeNote.relativePath}</p>
              {:else}
                <p class="text-sm text-danger">Ouvrez une note avant de l’exporter.</p>
              {/if}
            </div>
          </div>

          {#if pdfOptionsLoading}
            <p class="text-sm text-subtle" role="status">Recherche de Chromium…</p>
          {:else if pdfOptions}
            {#if pdfOptions.available}
              <p class="text-xs text-subtle">
                Rendu local avec <span class="font-medium text-foreground">{pdfOptions.browser}</span>.
                Aucun contenu n’est envoyé sur le réseau.
              </p>
              <label class="flex flex-col gap-1">
                <span class="text-xs font-medium text-subtle">Thème PDF</span>
                <select
                  bind:value={pdfThemeID}
                  class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none focus:border-accent"
                  disabled={busy}
                >
                  {#each pdfOptions.themes as theme (theme.id)}
                    <option value={theme.id}>{theme.name}</option>
                  {/each}
                </select>
              </label>
            {:else}
              <p class="rounded-md border border-danger/40 bg-background px-3 py-2 text-sm text-danger" role="status">
                {pdfOptions.unavailableReason}
              </p>
            {/if}

            {#if pdfOptions.warnings.length > 0}
              <div class="rounded-md border border-border bg-background px-3 py-2 text-xs text-subtle" role="status">
                <p class="font-medium text-foreground">Certains thèmes ont été ignorés :</p>
                <ul class="mt-1 list-disc space-y-1 pl-4">
                  {#each pdfOptions.warnings as warning}
                    <li>{warning}</li>
                  {/each}
                </ul>
              </div>
            {/if}
          {/if}

          {#if encrypted}
            <label class="flex items-start gap-2 border-l-2 border-danger px-3 py-2 text-xs leading-5 text-subtle">
              <input
                class="mt-1 h-3.5 w-3.5 accent-accent"
                type="checkbox"
                bind:checked={pdfPlaintextConfirmed}
                disabled={busy}
              />
              <span>Je confirme que ce PDF contiendra la note en clair et ne sera plus protégé par le chiffrement du coffre.</span>
            </label>
          {/if}
        </div>
      {/if}

      {#if error}
        <p class="mx-4 mb-3 rounded-md border border-danger/40 bg-background px-3 py-2 text-xs text-danger" role="alert">
          {error}
        </p>
      {/if}

      <footer class="flex items-center justify-between gap-2 border-t border-border bg-background px-4 py-3 text-xs text-subtle">
        <span aria-live="polite">
          {busy
            ? tab === 'pdf'
              ? 'Création du PDF en cours…'
              : 'Création de l’archive en cours…'
            : 'Ctrl+Entrée pour exporter'}
        </span>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="rounded-md border border-border bg-transparent px-3 py-1.5 text-sm text-subtle hover:bg-panel-muted hover:text-foreground disabled:opacity-50"
            onclick={close}
            disabled={busy}
          >
            Annuler
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-accent bg-accent px-3 py-1.5 text-sm font-medium text-accent-foreground hover:bg-accent-hover disabled:cursor-not-allowed disabled:opacity-50"
            onclick={() => void commit()}
            disabled={busy ||
              (tab === 'zip'
                ? selected.size === 0 || (encrypted && !zipPlaintextConfirmed)
                : Boolean(
                    pdfExportBlocker({
                      options: pdfOptions,
                      activeNotePath: activeNote?.relativePath ?? '',
                      encrypted,
                      plaintextConfirmed: pdfPlaintextConfirmed
                    })
                  ))}
          >
            <Download size={13} strokeWidth={2} aria-hidden="true" />
            {busy ? 'Export…' : tab === 'pdf' ? 'Enregistrer le PDF' : 'Exporter le ZIP'}
          </button>
        </div>
      </footer>
    </div>
  </div>
{/if}
