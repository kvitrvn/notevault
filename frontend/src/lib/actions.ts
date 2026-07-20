import type { Action } from 'svelte/action';

/**
 * Ferme un popover/menu quand l'utilisateur clique en dehors du nœud
 * attaché. À combiner avec un handler Escape pour les modals bloquants.
 *
 *   <div use:clickOutside={() => (open = false)}>…</div>
 *
 * L'action est désactivée tant que `enabled` est faux (utile pour ne pas
 * intercepter les clics avant l'ouverture effective). On utilise
 * `mousedown` plutôt que `click` pour fermer avant que le focus ne
 * change, évitant les race conditions avec les handlers d'éléments
 * focusables qui ferment eux-mêmes le popover.
 */
export const clickOutside: Action<HTMLElement, (() => void) | { handler: () => void; enabled?: boolean }> = (
  node,
  param
) => {
  let handler: () => void = () => {};
  let enabled = true;

  const resolve = (next: typeof param): void => {
    if (typeof next === 'function') {
      handler = next;
      enabled = true;
    } else if (next && typeof next === 'object') {
      handler = next.handler;
      enabled = next.enabled !== false;
    } else {
      handler = () => {};
      enabled = true;
    }
  };

  const onMouseDown = (event: MouseEvent): void => {
    if (!enabled) return;
    const target = event.target as Node | null;
    if (!target) return;
    if (node.contains(target)) return;
    handler();
  };

  resolve(param);
  document.addEventListener('mousedown', onMouseDown, true);

  return {
    update(next) {
      resolve(next);
    },
    destroy() {
      document.removeEventListener('mousedown', onMouseDown, true);
    }
  };
};