import { describe, expect, it } from 'vitest';
import { finalVaultPath, shouldShowVaultUnlock, validateVaultDraft } from './vault-manager';

describe('validateVaultDraft', () => {
  const valid = {
    name: 'Mes notes',
    parentPath: '/home/alice',
    encrypted: false,
    passphrase: '',
    confirmation: ''
  };

  it('accepts a readable Markdown vault by default', () => {
    expect(validateVaultDraft(valid)).toBe('');
    expect(finalVaultPath(valid.parentPath, valid.name)).toBe('/home/alice/Mes notes');
  });

  it('rejects unsafe names', () => {
    expect(validateVaultDraft({ ...valid, name: '../notes' })).toContain('non autorisé');
  });

  it('validates encrypted passphrases conditionally', () => {
    expect(validateVaultDraft({ ...valid, encrypted: true, passphrase: 'trop court', confirmation: 'trop court' })).toContain('12 caractères');
    expect(validateVaultDraft({ ...valid, encrypted: true, passphrase: 'phrase assez longue', confirmation: 'autre phrase longue' })).toContain('correspondent');
  });
});

describe('shouldShowVaultUnlock', () => {
  it('trusts the global locked mode while the detailed status is stale', () => {
    expect(shouldShowVaultUnlock('locked', 'unlocked')).toBe(true);
    expect(shouldShowVaultUnlock('ready', 'locked')).toBe(true);
    expect(shouldShowVaultUnlock('ready', 'unlocked')).toBe(false);
  });
});
