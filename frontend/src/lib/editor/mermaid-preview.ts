// Ordonnancement des aperçus Mermaid affichés sous les blocs ```mermaid.
//
// Crepe appelle `renderPreview` à chaque changement du texte du bloc, donc à
// chaque frappe. Sans régulation, une salve de saisie empile autant de rendus
// Mermaid qu'il y a de caractères tapés : la file de `renderMermaid` les
// sérialise mais ne les annule pas, l'aperçu affiché prend un retard croissant
// et le processeur tourne pour des états intermédiaires que personne ne verra.
//
// Difficulté : `renderPreview` ne dit pas de quel bloc vient la demande, et la
// fonction `applyPreview` est recréée à chaque appel — elle ne peut donc pas
// servir d'identité. On se rabat sur le bloc de code qui a le focus au moment
// de l'appel : pendant la frappe c'est forcément celui qu'on édite. Un rendu
// déclenché sans focus (montage du bloc, arrivée dans le viewport) n'a pas
// d'identité, et part immédiatement — c'est le premier affichage, il ne doit
// surtout pas être retardé.
//
// Règle d'abandon, volontairement stricte : une demande n'est abandonnée que
// si une demande plus récente vise le MÊME bloc focalisé. Dans tous les autres
// cas la demande en attente est lancée sans attendre. Un aperçu ne peut donc
// jamais rester bloqué sur « Rendu du diagramme… », ce qui serait le pire des
// symptômes — pire que le retard qu'on cherche à supprimer.

import { MERMAID_LANGUAGE } from './mermaid';

export type ApplyPreview = (value: null | string | HTMLElement) => void;

/** Signature de `renderPreview` attendue par la feature `codeMirror` de Crepe. */
export type PreviewRenderer = (
  language: string,
  content: string,
  applyPreview: ApplyPreview
) => void | null | string | HTMLElement;

export type MermaidPreviewOptions = {
  /** Rend le diagramme, ou rejette si la syntaxe est invalide. */
  render: (code: string) => Promise<string>;
  /** Contenu affiché à la place de l'aperçu quand le rendu échoue. */
  renderError: (error: unknown) => string | HTMLElement;
  /**
   * Bloc de code portant le focus, ou `null`. Sert d'identité de bloc : deux
   * appels partageant cette valeur viennent du même bloc.
   */
  focusedBlock: () => object | null;
  debounceMs?: number;
};

export const DEFAULT_PREVIEW_DEBOUNCE_MS = 300;

type Request = {
  key: object | null;
  content: string;
  apply: ApplyPreview;
};

type Pending = Request & {
  key: object;
  timer: ReturnType<typeof setTimeout>;
};

export function createMermaidPreviewRenderer(options: MermaidPreviewOptions): PreviewRenderer {
  const debounceMs = options.debounceMs ?? DEFAULT_PREVIEW_DEBOUNCE_MS;
  // Dernier rendu appliqué par bloc : permet de réafficher sans repasser par
  // Mermaid quand le texte revient à un état déjà rendu (retour arrière, ou
  // simple changement de langage qui refait tourner l'observateur de Crepe).
  const lastApplied = new WeakMap<object, { content: string; value: string }>();
  let pending: Pending | null = null;

  const start = (request: Request): void => {
    void options
      .render(request.content)
      .then((svg) => {
        if (request.key) lastApplied.set(request.key, { content: request.content, value: svg });
        request.apply(svg);
      })
      // `applyPreview` doit être appelé dans tous les cas, erreur comprise :
      // c'est ce qui sort le panneau de son état de chargement.
      .catch((error) => request.apply(options.renderError(error)));
  };

  const takePending = (): Pending | null => {
    const taken = pending;
    if (taken) {
      clearTimeout(taken.timer);
      pending = null;
    }
    return taken;
  };

  return (language, content, apply) => {
    if (language.toLowerCase() !== MERMAID_LANGUAGE || content.trim() === '') return null;

    const key = options.focusedBlock();

    if (key !== null) {
      const previous = lastApplied.get(key);
      if (previous !== undefined && previous.content === content) {
        // L'état affiché est déjà le bon : une demande en attente sur ce même
        // bloc est périmée par construction.
        if (pending?.key === key) takePending();
        return previous.value;
      }
    }

    const superseded = pending !== null && key !== null && pending.key === key;
    const previous = takePending();
    if (previous !== null && !superseded) start(previous);

    if (key === null) {
      start({ key, content, apply });
      return;
    }

    const request: Request & { key: object } = { key, content, apply };
    pending = {
      ...request,
      timer: setTimeout(() => {
        pending = null;
        start(request);
      }, debounceMs)
    };
  };
}
