// Comparaisons « est-ce que ça a réellement changé ? ».
//
// Ces deux questions sont posées sur les chemins les plus chauds de l'éditeur,
// et la réponse naïve y coûte un parcours du document entier :
//
//   - « la note a-t-elle été modifiée depuis la dernière sauvegarde ? » était
//     répondu par un `JSON.stringify` du contenu complet, plusieurs fois par
//     salve de frappe (indicateur de modification, autosave, buffer de
//     récupération, sauvegarde). Sur une note de 240 Kio c'est ~0,34 ms par
//     appel et autant d'allocations à collecter ;
//   - « la liste des titres connus a-t-elle changé ? » n'était pas posée du
//     tout : un `Set` d'identité neuve était recréé à chaque rafraîchissement
//     du coffre, ce qui déclenchait une reconstruction complète des décorations
//     wiki-link (~137 ms sur un document de 4 000 blocs).
//
// Dans les deux cas le cas nominal est « rien n'a changé ». Ces comparaisons
// sont écrites pour que ce cas-là soit le moins cher possible.

import type { domain } from '../../wailsjs/go/models';

export type NoteIdentity = {
  title: string;
  content: string;
  tags: string[];
};

/**
 * Capture les seuls champs qui rendent une note « modifiée ». `tags` est copié
 * car le tableau de la note est muté en place par l'éditeur de tags.
 */
export function noteIdentity(note: domain.Note): NoteIdentity {
  return {
    title: note.title,
    content: note.content,
    tags: [...(note.tags ?? [])]
  };
}

/**
 * Compare deux identités. Le champ `content` est comparé par `===` : quand rien
 * n'a bougé, les deux chaînes sont la même référence et le moteur répond sans
 * lire un seul caractère — c'est tout l'intérêt par rapport à une sérialisation.
 */
export function sameNoteIdentity(a: NoteIdentity | null, b: NoteIdentity | null): boolean {
  if (a === b) return true;
  if (a === null || b === null) return false;
  if (a.title !== b.title) return false;
  if (a.content !== b.content) return false;
  if (a.tags.length !== b.tags.length) return false;
  return a.tags.every((tag, i) => tag === b.tags[i]);
}

/**
 * Compare une note à une identité sans rien allouer.
 *
 * C'est la forme utilisée sur le chemin chaud : `sameNoteIdentity(noteIdentity(note), saved)`
 * donnerait le même résultat mais copierait les tags à chaque comparaison,
 * alors que la réponse est « identique » dans l'immense majorité des cas.
 */
export function noteMatchesIdentity(
  note: domain.Note | null,
  identity: NoteIdentity | null
): boolean {
  if (note === null || identity === null) return note === null && identity === null;
  if (note.title !== identity.title) return false;
  if (note.content !== identity.content) return false;
  const tags = note.tags ?? [];
  if (tags.length !== identity.tags.length) return false;
  return tags.every((tag, i) => tag === identity.tags[i]);
}

/** Vrai si les deux ensembles contiennent exactement les mêmes titres. */
export function sameTitleSet(a: Set<string>, b: Set<string>): boolean {
  if (a === b) return true;
  if (a.size !== b.size) return false;
  for (const title of a) {
    if (!b.has(title)) return false;
  }
  return true;
}
