export type ChatProvider = 'ollama' | 'openai' | 'mistral' | 'openrouter';

export const REMOTE_CHAT_PROVIDERS: ChatProvider[] = ['openai', 'mistral', 'openrouter'];

export function isRemoteProvider(provider: ChatProvider): boolean {
  return provider !== 'ollama';
}

export function availableProvider(provider: ChatProvider, keyringAvailable: boolean): ChatProvider {
  return isRemoteProvider(provider) && !keyringAvailable ? 'ollama' : provider;
}

export function modelForProvider(models: Record<string, string> | undefined, provider: ChatProvider): string {
  return models?.[provider] ?? '';
}

export function hasStoredAPIKey(providers: string[] | undefined, provider: ChatProvider): boolean {
  return providers?.includes(provider) ?? false;
}

export function updateStoredAPIKey(
  providers: ChatProvider[],
  provider: ChatProvider,
  stored: boolean
): ChatProvider[] {
  const withoutProvider = providers.filter((item) => item !== provider);
  return stored ? [...withoutProvider, provider] : withoutProvider;
}
