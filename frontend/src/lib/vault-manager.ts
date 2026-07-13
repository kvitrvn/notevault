export type VaultDraft = {
  name: string;
  parentPath: string;
  encrypted: boolean;
  passphrase: string;
  confirmation: string;
};

export function shouldShowVaultUnlock(applicationMode: string, vaultState?: string): boolean {
  return applicationMode === 'locked' || vaultState === 'locked';
}

export function finalVaultPath(parentPath: string, name: string): string {
  const separator = parentPath.includes('\\') && !parentPath.includes('/') ? '\\' : '/';
  return `${parentPath.replace(/[\\/]+$/, '')}${parentPath ? separator : ''}${name.trim()}`;
}

export function validateVaultDraft(draft: VaultDraft): string {
  const name = draft.name.trim();
  if (name.length < 1 || Array.from(name).length > 80) {
    return 'Le nom doit contenir entre 1 et 80 caractères.';
  }
  if (name === '.' || name === '..' || /[\\/]/.test(name) || /[\u0000-\u001f\u007f]/.test(name)) {
    return 'Le nom contient un caractère non autorisé.';
  }
  if (!draft.parentPath.trim()) {
    return 'Choisissez un emplacement existant.';
  }
  if (draft.encrypted) {
    if (Array.from(draft.passphrase).length < 12) {
      return 'La phrase secrète doit contenir au moins 12 caractères.';
    }
    if (draft.passphrase !== draft.confirmation) {
      return 'Les deux phrases secrètes ne correspondent pas.';
    }
  }
  return '';
}
