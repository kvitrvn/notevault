<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import Eye from '@lucide/svelte/icons/eye';
  import FileText from '@lucide/svelte/icons/file-text';
  import Hash from '@lucide/svelte/icons/hash';
  import Loader2 from '@lucide/svelte/icons/loader';
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Search from '@lucide/svelte/icons/search';
  import Send from '@lucide/svelte/icons/send';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import X from '@lucide/svelte/icons/x';

  import { chat, domain, vault } from '../../wailsjs/go/models';
  import {
    DeleteChatAPIKey,
    GetChatSettings,
    PrepareChat,
    ResetChatConversation,
    SendPreparedChat,
    StoreChatAPIKey,
    UpdateChatPreferences
  } from '../../wailsjs/go/main/App';
  import { resolveChatPaths } from '../lib/chat-selection';
  import {
    availableProvider,
    hasStoredAPIKey,
    isRemoteProvider,
    modelForProvider,
    updateStoredAPIKey,
    type ChatProvider
  } from '../lib/chat-settings';
  import ChatMarkdown from './ChatMarkdown.svelte';

  type ChatMessage = {
    id: number;
    role: 'user' | 'assistant';
    content: string;
    citations: chat.Citation[];
  };

  type Props = {
    open: boolean;
    notes: domain.NoteSummary[];
    availableTags: vault.TagCount[];
    currentPath: string;
    encrypted: boolean;
    beforePrepare: () => Promise<boolean>;
    onOpenNote: (path: string) => void;
    onClose: () => void;
  };

  let { open, notes, availableTags, currentPath, encrypted, beforePrepare, onOpenNote, onClose }: Props = $props();

  let provider: ChatProvider = $state('ollama');
  let model = $state('');
  let apiKey = $state('');
  let rememberAPIKey = $state(false);
  let chatModels: Record<string, string> = $state({});
  let storedAPIKeyProviders: ChatProvider[] = $state([]);
  let keyringAvailable = $state(false);
  let settingsLoaded = $state(false);
  let settingsLoading = $state(false);
  let noteFilter = $state('');
  let manualSelectedPaths: string[] = $state([]);
  let excludedPaths: string[] = $state([]);
  let selectedTags: string[] = $state([]);
  let question = $state('');
  let pendingQuestion = $state('');
  let preview: chat.Preview | null = $state(null);
  let conversationID = $state('');
  let messages: ChatMessage[] = $state([]);
  let busy: '' | 'preparing' | 'sending' = $state('');
  let error = $state('');
  let sequence = 0;
  let transcript: HTMLDivElement | undefined = $state();
  let initializedForOpen = false;
  let previousProvider: ChatProvider = 'ollama';

  const filteredNotes = $derived(
    notes.filter((note) => {
      const query = noteFilter.trim().toLocaleLowerCase('fr-FR');
      return !query || note.title.toLocaleLowerCase('fr-FR').includes(query) || note.relativePath.toLocaleLowerCase('fr-FR').includes(query);
    })
  );
  const remote = $derived(isRemoteProvider(provider));
  const hasStoredKey = $derived(hasStoredAPIKey(storedAPIKeyProviders, provider));
  const selectedPaths = $derived(resolveChatPaths(notes, manualSelectedPaths, selectedTags, excludedPaths));

  $effect(() => {
    if (!open) {
      initializedForOpen = false;
      apiKey = '';
      rememberAPIKey = false;
      return;
    }
    if (!initializedForOpen) {
      initializedForOpen = true;
      if (selectedPaths.length === 0 && currentPath) manualSelectedPaths = [currentPath];
    }
    if (!settingsLoaded && !settingsLoading) void loadChatSettings();
  });

  $effect(() => {
    provider;
    if (!settingsLoaded || provider === previousProvider) return;
    previousProvider = provider;
    model = modelForProvider(chatModels, provider);
    apiKey = '';
    rememberAPIKey = false;
    error = '';
  });

  async function loadChatSettings(): Promise<void> {
    settingsLoading = true;
    try {
      const settings = await GetChatSettings();
      chatModels = settings.models ?? {};
      keyringAvailable = settings.keyringAvailable;
      storedAPIKeyProviders = (settings.providersWithAPIKey ?? []) as ChatProvider[];
      const requested = (settings.provider || 'ollama') as ChatProvider;
      provider = availableProvider(requested, keyringAvailable);
      previousProvider = provider;
      model = modelForProvider(chatModels, provider);
      settingsLoaded = true;
    } catch (err) {
      error = `Impossible de charger les réglages du chat : ${String(err)}`;
    } finally {
      settingsLoaded = true;
      settingsLoading = false;
    }
  }

  function togglePath(path: string): void {
    const note = notes.find((item) => item.relativePath === path);
    const selectedByTag = selectedTags.some((tag) => (note?.tags ?? []).includes(tag));
    if (selectedPaths.includes(path)) {
      manualSelectedPaths = manualSelectedPaths.filter((item) => item !== path);
      if (selectedByTag && !excludedPaths.includes(path)) excludedPaths = [...excludedPaths, path];
      return;
    }
    excludedPaths = excludedPaths.filter((item) => item !== path);
    if (!manualSelectedPaths.includes(path)) manualSelectedPaths = [...manualSelectedPaths, path];
  }

  function selectVisible(): void {
    const visiblePaths = filteredNotes.map((note) => note.relativePath);
    manualSelectedPaths = [...new Set([...manualSelectedPaths, ...visiblePaths])];
    excludedPaths = excludedPaths.filter((path) => !visiblePaths.includes(path));
  }

  function toggleTag(tag: string): void {
    selectedTags = selectedTags.includes(tag)
      ? selectedTags.filter((item) => item !== tag)
      : [...selectedTags, tag];
  }

  async function prepare(): Promise<void> {
    error = '';
    const trimmed = question.trim();
    if (!trimmed) {
      error = 'Écrivez une question.';
      return;
    }
    if (!model.trim()) {
      error = 'Indiquez le modèle à utiliser.';
      return;
    }
    if (remote && !keyringAvailable) {
      error = 'Les fournisseurs distants sont désactivés car le trousseau système est verrouillé ou indisponible.';
      return;
    }
    if (selectedPaths.length === 0) {
      error = 'Sélectionnez au moins une note.';
      return;
    }
    if (selectedPaths.length > 50) {
      error = `La sélection contient ${selectedPaths.length} notes. Le maximum est de 50.`;
      return;
    }
    if (!(await beforePrepare())) {
      error = 'La note courante doit être enregistrée avant la préparation.';
      return;
    }
    busy = 'preparing';
    try {
      const result = await PrepareChat({
        conversationID,
        notePaths: selectedPaths,
        question: trimmed,
        provider,
        model: model.trim()
      });
      preview = result;
      conversationID = result.conversationID;
      pendingQuestion = trimmed;
      await UpdateChatPreferences(provider, model.trim());
      chatModels = { ...chatModels, [provider]: model.trim() };
    } catch (err) {
      error = String(err);
    } finally {
      busy = '';
    }
  }

  async function sendPrepared(): Promise<void> {
    if (!preview) return;
    if (remote && !keyringAvailable) {
      error = 'Les fournisseurs distants sont désactivés car le trousseau système est verrouillé ou indisponible.';
      return;
    }
    if (remote && !apiKey.trim() && !hasStoredKey) {
      error = 'Saisissez une clé API ou enregistrez-en une dans le trousseau système.';
      return;
    }
    error = '';
    busy = 'sending';
    try {
      const oneTimeAPIKey = apiKey.trim();
      if (remote && oneTimeAPIKey && rememberAPIKey) {
        await StoreChatAPIKey(provider, oneTimeAPIKey);
        storedAPIKeyProviders = updateStoredAPIKey(storedAPIKeyProviders, provider, true);
      }
      const response = await SendPreparedChat({ previewID: preview.id, apiKey: oneTimeAPIKey });
      messages = [
        ...messages,
        { id: ++sequence, role: 'user', content: pendingQuestion, citations: [] },
        {
          id: ++sequence,
          role: 'assistant',
          content: response.answer,
          citations: response.citations ?? []
        }
      ];
      preview = null;
      question = '';
      pendingQuestion = '';
      requestAnimationFrame(() => transcript?.scrollTo({ top: transcript.scrollHeight, behavior: 'smooth' }));
    } catch (err) {
      error = String(err);
    } finally {
      apiKey = '';
      rememberAPIKey = false;
      busy = '';
    }
  }

  async function forgetAPIKey(): Promise<void> {
    error = '';
    try {
      await DeleteChatAPIKey(provider);
      storedAPIKeyProviders = updateStoredAPIKey(storedAPIKeyProviders, provider, false);
      apiKey = '';
      rememberAPIKey = false;
    } catch (err) {
      error = String(err);
    }
  }

  async function resetConversation(): Promise<void> {
    const previous = conversationID;
    conversationID = '';
    preview = null;
    pendingQuestion = '';
    messages = [];
    error = '';
    if (!previous) return;
    try {
      await ResetChatConversation(previous);
    } catch (err) {
      error = String(err);
    }
  }

  function providerLabel(value: ChatProvider): string {
    if (value === 'openai') return 'OpenAI';
    if (value === 'mistral') return 'Mistral';
    if (value === 'openrouter') return 'OpenRouter';
    return 'Ollama local';
  }
</script>

<svelte:window
  onkeydown={(event) => {
    if (open && event.key === 'Escape' && busy === '') {
      if (preview) preview = null;
      else onClose();
    }
  }}
/>

{#if open}
  <button
    class="fixed inset-x-0 bottom-0 top-[5.75rem] z-30 bg-black/50 xl:hidden"
    type="button"
    aria-label="Fermer le chat"
    onclick={onClose}
  ></button>
  <aside
    class="fixed bottom-0 right-0 top-[5.75rem] z-40 flex min-h-0 w-full max-w-[34rem] min-w-0 flex-col overflow-hidden border-l border-border bg-panel shadow-lg xl:static xl:z-auto xl:h-full xl:w-auto xl:max-w-none xl:shadow-none"
    aria-label="Discussion avec les notes"
  >
    <header class="flex h-12 shrink-0 items-center justify-between gap-3 border-b border-border px-3">
      <div class="flex min-w-0 items-center gap-2">
        <MessageSquare size={16} strokeWidth={2} class="shrink-0 text-accent" aria-hidden="true" />
        <h2 class="truncate text-sm font-semibold">Discuter avec les notes</h2>
      </div>
      <div class="flex items-center gap-1">
        <button
          class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-background px-2 text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          onclick={() => void resetConversation()}
          disabled={busy !== '' || (!conversationID && messages.length === 0)}
        >
          <RotateCcw size={12} strokeWidth={2} aria-hidden="true" />
          Nouveau
        </button>
        <button
          class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-background text-subtle hover:bg-panel-muted hover:text-foreground"
          type="button"
          aria-label="Fermer le chat"
          onclick={onClose}
        >
          <X size={14} strokeWidth={2} aria-hidden="true" />
        </button>
      </div>
    </header>

    {#if encrypted}
      <div class="m-4 border-l-2 border-danger pl-3" role="status">
        <p class="text-sm font-medium text-foreground">Chat désactivé pour ce coffre chiffré</p>
        <p class="mt-1 text-xs leading-5 text-subtle">
          La première version ne crée aucune donnée dérivée en clair à partir d’un coffre chiffré.
        </p>
      </div>
    {:else}
      <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div class="max-h-[46%] shrink-0 overflow-y-auto overscroll-contain border-b border-border bg-background px-3 py-3">
          <div class="grid grid-cols-2 gap-2">
            <label class="text-xs font-medium text-foreground">
              Fournisseur
              <select
                class="mt-1 h-9 w-full rounded-md border border-border-strong bg-panel px-2 text-sm text-foreground"
                bind:value={provider}
                disabled={busy !== '' || conversationID !== ''}
              >
                <option value="ollama">Ollama local</option>
                <option value="openai" disabled={!keyringAvailable}>OpenAI</option>
                <option value="mistral" disabled={!keyringAvailable}>Mistral</option>
                <option value="openrouter" disabled={!keyringAvailable}>OpenRouter</option>
              </select>
            </label>
            <label class="text-xs font-medium text-foreground">
              Modèle
              <input
                class="mt-1 h-9 w-full rounded-md border border-border-strong bg-panel px-2 text-sm text-foreground placeholder:text-faint"
                bind:value={model}
                placeholder={provider === 'ollama' ? 'ex. qwen3:4b' : 'Identifiant du modèle'}
                disabled={busy !== '' || conversationID !== ''}
                maxlength="120"
              />
            </label>
          </div>
          {#if settingsLoaded && !keyringAvailable}
            <p class="mt-2 border-l-2 border-danger pl-2 text-xs leading-5 text-danger" role="status">
              Trousseau système verrouillé ou indisponible. Les fournisseurs distants sont désactivés ; Ollama reste utilisable.
            </p>
          {/if}
          {#if remote}
            <label class="mt-2 block text-xs font-medium text-foreground" for="chat-api-key">
              Clé API
              <input
                id="chat-api-key"
                class="mt-1 h-9 w-full rounded-md border border-border-strong bg-panel px-2 font-mono text-sm text-foreground placeholder:text-faint"
                bind:value={apiKey}
                type="password"
                autocomplete="off"
                placeholder={`Clé ${providerLabel(provider)}`}
                disabled={busy !== '' || !keyringAvailable}
              />
            </label>
            <div class="mt-2 flex items-start justify-between gap-3">
              <label class="flex min-w-0 items-start gap-2 text-xs leading-5 text-subtle">
                <input
                  class="mt-0.5"
                  type="checkbox"
                  bind:checked={rememberAPIKey}
                  disabled={busy !== '' || !keyringAvailable || !apiKey.trim()}
                />
                <span>Mémoriser dans le trousseau système</span>
              </label>
              {#if hasStoredKey}
                <button
                  class="shrink-0 text-xs text-subtle underline decoration-border-strong underline-offset-2 hover:text-foreground"
                  type="button"
                  onclick={() => void forgetAPIKey()}
                  disabled={busy !== '' || !keyringAvailable}
                >Oublier la clé</button>
              {/if}
            </div>
            {#if hasStoredKey}
              <p class="mt-1 inline-flex items-center gap-1 text-[11px] text-subtle" role="status">
                <ShieldCheck size={12} aria-hidden="true" />
                Clé enregistrée dans le trousseau
              </p>
            {:else}
              <p class="mt-1 text-[11px] text-faint">Sans mémorisation, la saisie sert uniquement au prochain envoi.</p>
            {/if}
          {/if}

          <details class="mt-3" open>
            <summary class="cursor-pointer text-xs font-medium text-foreground">
              Sources sélectionnées ({selectedPaths.length})
            </summary>
            <div class="mt-2 rounded-md border border-border bg-panel">
              {#if availableTags.length > 0}
                <div class="border-b border-border px-2 py-2">
                  <div class="mb-1.5 flex items-center justify-between gap-2">
                    <span class="inline-flex items-center gap-1 text-xs font-medium text-foreground">
                      <Hash size={12} aria-hidden="true" /> Ajouter par tags
                    </span>
                    {#if selectedTags.length > 0}
                      <button
                        class="text-[11px] text-subtle hover:text-foreground"
                        type="button"
                        onclick={() => (selectedTags = [])}
                        disabled={busy !== ''}
                      >Effacer</button>
                    {/if}
                  </div>
                  <div class="flex max-h-24 flex-wrap gap-1 overflow-y-auto" role="group" aria-label="Sélectionner des tags">
                    {#each availableTags as item (item.tag)}
                      <button
                        class={selectedTags.includes(item.tag)
                          ? 'inline-flex h-7 items-center gap-1 rounded-md border border-accent bg-accent/15 px-2 text-xs text-accent'
                          : 'inline-flex h-7 items-center gap-1 rounded-md border border-border bg-background px-2 text-xs text-subtle hover:bg-panel-muted hover:text-foreground'}
                        type="button"
                        aria-pressed={selectedTags.includes(item.tag)}
                        onclick={() => toggleTag(item.tag)}
                        disabled={busy !== ''}
                      >
                        {item.tag}
                        <span class="text-[10px] text-faint">{item.count}</span>
                      </button>
                    {/each}
                  </div>
                  <p class="mt-1.5 text-[11px] text-faint">Plusieurs tags additionnent leurs notes. Décochez ensuite les exceptions ci-dessous.</p>
                </div>
              {/if}
              <label class="flex h-9 items-center gap-2 border-b border-border px-2">
                <Search size={12} class="text-faint" aria-hidden="true" />
                <span class="sr-only">Filtrer les notes</span>
                <input
                  class="min-w-0 flex-1 border-0 bg-transparent text-xs text-foreground outline-none placeholder:text-faint"
                  bind:value={noteFilter}
                  placeholder="Filtrer les notes…"
                />
                <button class="text-xs text-subtle hover:text-foreground" type="button" onclick={selectVisible}>Tout</button>
              </label>
              <ul class="max-h-36 overflow-y-auto py-1">
                {#each filteredNotes as note (note.relativePath)}
                  <li>
                    <label class="flex cursor-pointer items-center gap-2 px-2 py-1.5 hover:bg-panel-muted">
                      <input
                        type="checkbox"
                        checked={selectedPaths.includes(note.relativePath)}
                        onchange={() => togglePath(note.relativePath)}
                        disabled={busy !== ''}
                      />
                      <span class="min-w-0 flex-1">
                        <span class="block truncate text-xs text-foreground">{note.title || 'Sans titre'}</span>
                        <span class="block truncate text-[11px] text-faint">{note.relativePath}</span>
                      </span>
                    </label>
                  </li>
                {/each}
              </ul>
              {#if selectedPaths.length > 50}
                <p class="border-t border-danger/40 px-2 py-1.5 text-[11px] text-danger" role="status">
                  {selectedPaths.length} notes sélectionnées — maximum 50.
                </p>
              {/if}
            </div>
          </details>
        </div>

        <div
          bind:this={transcript}
          class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3"
          role="log"
          aria-label="Messages de la discussion"
          aria-live="polite"
        >
          {#if messages.length === 0 && !preview}
            <div class="border-l-2 border-accent pl-3 text-sm leading-6 text-subtle">
              <p>La recherche Amoxtli et l’anonymisation s’exécutent localement.</p>
              <p class="mt-1 text-xs text-faint">Le modèle go-anon nécessaire (environ 200 Mio) est importé et vérifié au premier aperçu.</p>
            </div>
          {/if}

          <div class="flex flex-col gap-4">
            {#each messages as message (message.id)}
              <article class={message.role === 'user' ? 'ml-8 border-r-2 border-border-strong pr-3 text-right' : 'mr-4 border-l-2 border-accent pl-3'}>
                {#if message.role === 'assistant'}
                  <ChatMarkdown markdown={message.content} />
                {:else}
                  <p class="whitespace-pre-wrap break-words text-sm leading-6 text-foreground">{message.content}</p>
                {/if}
                {#if message.citations.length > 0}
                  <ul class="mt-2 flex flex-col gap-1" aria-label="Sources de la réponse">
                    {#each message.citations as citation (`${message.id}-${citation.sourceID}`)}
                      <li>
                        <button
                          class="flex w-full min-w-0 items-center gap-1.5 rounded-md border border-border bg-background px-2 py-1 text-left text-xs text-subtle hover:bg-panel-muted hover:text-foreground"
                          type="button"
                          onclick={() => onOpenNote(citation.path)}
                          title={citation.path}
                        >
                          <FileText size={11} class="shrink-0" aria-hidden="true" />
                          <span class="shrink-0 font-medium text-foreground">[{citation.sourceID}]</span>
                          <span class="truncate">{citation.title} — {citation.section}</span>
                        </button>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </article>
            {/each}
          </div>

          {#if preview}
            <section class="mt-4 border border-border-strong bg-background" aria-labelledby="chat-preview-title">
              <header class="flex items-center gap-2 border-b border-border px-3 py-2">
                <Eye size={14} class="text-accent" aria-hidden="true" />
                <h3 id="chat-preview-title" class="text-sm font-semibold">Aperçu avant envoi</h3>
              </header>
              <div class="max-h-[45vh] overflow-y-auto px-3 py-3">
                <p class="text-xs font-medium text-foreground">Question anonymisée</p>
                <p class="mt-1 whitespace-pre-wrap break-words rounded-md border border-border bg-panel px-2 py-2 text-xs leading-5 text-foreground">{preview.anonymizedQuestion}</p>

                <div class="mt-3 flex flex-col gap-2">
                  {#each preview.excerpts as excerpt (excerpt.sourceID)}
                    <details class="rounded-md border border-border bg-panel">
                      <summary class="cursor-pointer px-2 py-2 text-xs text-foreground">
                        <strong>[{excerpt.sourceID}]</strong> {excerpt.title} — {excerpt.section}
                      </summary>
                      <div class="border-t border-border px-2 py-2">
                        <p class="text-[11px] font-medium text-subtle">Original local</p>
                        <pre class="mt-1 max-h-28 overflow-auto whitespace-pre-wrap break-words font-sans text-xs leading-5 text-foreground">{excerpt.original}</pre>
                        <p class="mt-2 text-[11px] font-medium text-subtle">Texte envoyé</p>
                        <pre class="mt-1 max-h-28 overflow-auto whitespace-pre-wrap break-words font-sans text-xs leading-5 text-foreground">{excerpt.anonymized}</pre>
                        {#if excerpt.entities.length > 0}
                          <p class="mt-2 text-[11px] text-faint">
                            {excerpt.entities.length} entité{excerpt.entities.length > 1 ? 's' : ''} détectée{excerpt.entities.length > 1 ? 's' : ''}
                          </p>
                        {/if}
                      </div>
                    </details>
                  {/each}
                </div>

                <details class="mt-3">
                  <summary class="cursor-pointer text-xs text-subtle">Voir la charge textuelle exacte</summary>
                  <pre class="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-md border border-border bg-panel p-2 font-mono text-[11px] leading-5 text-foreground">{preview.outboundText}</pre>
                </details>
              </div>
              <footer class="flex items-center justify-between gap-2 border-t border-border px-3 py-2">
                <span class="inline-flex items-center gap-1 text-[11px] text-subtle">
                  <ShieldCheck size={12} aria-hidden="true" />
                  {remote ? `Envoi vers ${providerLabel(provider)}` : 'Envoi vers Ollama local'}
                </span>
                <div class="flex items-center gap-2">
                  <button class="h-8 rounded-md border border-border px-2 text-xs text-subtle hover:bg-panel-muted" type="button" onclick={() => (preview = null)} disabled={busy !== ''}>Modifier</button>
                  <button class="inline-flex h-8 items-center gap-1.5 rounded-md border border-accent bg-accent px-3 text-xs font-medium text-accent-foreground hover:bg-accent-hover" type="button" onclick={() => void sendPrepared()} disabled={busy !== ''}>
                    {#if busy === 'sending'}<Loader2 size={12} class="animate-spin" aria-hidden="true" />{:else}<Check size={12} aria-hidden="true" />{/if}
                    {busy === 'sending' ? 'Envoi…' : 'Valider et envoyer'}
                  </button>
                </div>
              </footer>
            </section>
          {/if}
        </div>

        {#if error}
          <p class="mx-3 mb-2 border-l-2 border-danger pl-2 text-xs leading-5 text-danger" role="alert">{error}</p>
        {/if}

        <form
          class="shrink-0 border-t border-border bg-background p-3"
          onsubmit={(event) => { event.preventDefault(); void prepare(); }}
        >
          <label for="chat-question" class="sr-only">Question pour les notes</label>
          <textarea
            id="chat-question"
            class="block min-h-20 w-full resize-y rounded-md border border-border-strong bg-panel px-3 py-2 text-sm leading-5 text-foreground outline-none placeholder:text-faint focus:border-accent"
            bind:value={question}
            placeholder="Posez une question sur la sélection…"
            maxlength="4000"
            disabled={busy !== '' || preview !== null}
            onkeydown={(event) => {
              if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
                event.preventDefault();
                void prepare();
              }
            }}
          ></textarea>
          <div class="mt-2 flex items-center justify-between gap-2">
            <span class="text-[11px] text-faint">Ctrl+Entrée prépare l’aperçu</span>
            <button
              class="inline-flex h-9 items-center gap-1.5 rounded-md border border-accent bg-accent px-3 text-xs font-medium text-accent-foreground hover:bg-accent-hover disabled:opacity-60"
              type="submit"
              disabled={busy !== '' || preview !== null || !question.trim() || !model.trim() || selectedPaths.length === 0 || selectedPaths.length > 50 || (remote && !keyringAvailable)}
            >
              {#if busy === 'preparing'}<Loader2 size={13} class="animate-spin" aria-hidden="true" />{:else}<Send size={13} aria-hidden="true" />{/if}
              {busy === 'preparing' ? 'Recherche et anonymisation…' : 'Préparer l’aperçu'}
            </button>
          </div>
        </form>
      </div>
    {/if}
  </aside>
{/if}
