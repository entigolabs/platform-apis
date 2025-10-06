import ExecutionEnvironment from '@docusaurus/ExecutionEnvironment';

// Type definitions for runtime config
interface PostHogRuntimeConfig {
  apiKey: string;
  apiHost: string;
  enabled: boolean;
}

// Extend Window interface
declare global {
  interface Window {
    POSTHOG_CONFIG?: PostHogRuntimeConfig;
    posthogConfig?: {
      apiKey: string;
      apiHost: string;
      appUrl: string;
      enableInDevelopment: boolean;
    };
  }
}

export default (function (): null {
  if (!ExecutionEnvironment.canUseDOM) {
    return null;
  }

  // Wait for runtime config to load
  function initPostHog(): void {
    if (!window.POSTHOG_CONFIG) {
      setTimeout(initPostHog, 100);
      return;
    }

    const config = window.POSTHOG_CONFIG;

    // Only proceed if enabled and has API key
    if (!config.enabled || !config.apiKey) {
      console.log('PostHog disabled or no API key provided');
      return;
    }

    // Set global config for posthog-docusaurus plugin to use
    window.posthogConfig = {
      apiKey: config.apiKey,
      apiHost: config.apiHost,
      appUrl: 'https://eu.posthog.com', // For PostHog UI
      enableInDevelopment: false,
    };

    console.log('PostHog runtime config loaded:', {
      apiHost: config.apiHost,
      enabled: config.enabled,
    });
  }

  initPostHog();

  return null;
})();