import { describe, expect, it } from 'vitest';

import {
  availableProvider,
  hasStoredAPIKey,
  modelForProvider,
  updateStoredAPIKey,
  type ChatProvider
} from './chat-settings';

describe('chat settings', () => {
  it('restores a distinct model for each provider', () => {
    const models = { ollama: 'qwen3:4b', openai: 'gpt-test', mistral: 'mistral-test' };
    expect(modelForProvider(models, 'ollama')).toBe('qwen3:4b');
    expect(modelForProvider(models, 'openai')).toBe('gpt-test');
    expect(modelForProvider(models, 'openrouter')).toBe('');
  });

  it('falls back to Ollama when the keyring blocks remote providers', () => {
    expect(availableProvider('openai', false)).toBe('ollama');
    expect(availableProvider('mistral', true)).toBe('mistral');
  });

  it('tracks key replacement and forgetting without exposing a value', () => {
    let providers: ChatProvider[] = [];
    providers = updateStoredAPIKey(providers, 'openai', true);
    providers = updateStoredAPIKey(providers, 'openai', true);
    expect(providers).toEqual(['openai']);
    expect(hasStoredAPIKey(providers, 'openai')).toBe(true);

    providers = updateStoredAPIKey(providers, 'openai', false);
    expect(providers).toEqual([]);
    expect(hasStoredAPIKey(providers, 'openai')).toBe(false);
  });
});
