import type { vault } from '../../wailsjs/go/models';

type PDFExportState = {
  options: vault.PDFExportOptionsInfo | null;
  activeNotePath: string;
  encrypted: boolean;
  plaintextConfirmed: boolean;
};

export function pdfExportBlocker(state: PDFExportState): string {
  if (!state.options) return 'Les options PDF sont en cours de chargement.';
  if (!state.options.available) {
    return state.options.unavailableReason || 'L’export PDF est indisponible.';
  }
  if (!state.activeNotePath) return 'Ouvrez une note avant de l’exporter.';
  if (state.encrypted && !state.plaintextConfirmed) {
    return 'Confirmez que le PDF contiendra la note en clair.';
  }
  if (state.options.themes.length === 0) return 'Aucun thème PDF valide n’est disponible.';
  return '';
}

export function canCloseExportDialog(busy: boolean): boolean {
  return !busy;
}
