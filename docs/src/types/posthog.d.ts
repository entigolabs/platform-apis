// Type definitions for PostHog runtime configuration

export interface PostHogRuntimeConfig {
  apiKey: string;
  apiHost: string;
  enabled: boolean;
}

export interface PostHogGlobalConfig {
  apiKey: string;
  apiHost: string;
  appUrl: string;
  enableInDevelopment: boolean;
}

declare global {
  interface Window {
    POSTHOG_CONFIG?: PostHogRuntimeConfig;
    posthogConfig?: PostHogGlobalConfig;
    posthog?: {
      init: (apiKey: string, config: Record<string, any>) => void;
      capture: (event: string, properties?: Record<string, any>) => void;
      identify: (userId: string, properties?: Record<string, any>) => void;
      reset: () => void;
      isFeatureEnabled: (flag: string) => boolean;
      onFeatureFlags: (callback: (flags: string[]) => void) => void;
      // Add other PostHog methods as needed
    };
  }
}

export {};