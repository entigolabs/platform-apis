import ExecutionEnvironment from '@docusaurus/ExecutionEnvironment';

// Type definitions for runtime config
interface PostHogRuntimeConfig {
  apiKey: string;
  apiHost: string;
  enabled: boolean;
}

declare global {
  interface Window {
    POSTHOG_CONFIG?: PostHogRuntimeConfig;
    posthog?: any;
  }
}

function isValidPostHogKey(key: string): boolean {
  if (!key) return false;
  return key.startsWith('phc_') && key.length >= 20;
}

function createPostHogStub() {
  return {
    init: () => {},
    capture: () => {},
    identify: () => {},
    alias: () => {},
    register: () => {},
    register_once: () => {},
    unregister: () => {},
    reset: () => {},
    isFeatureEnabled: () => false,
    onFeatureFlags: () => {},
    getFeatureFlag: () => undefined,
    getFeatureFlagPayload: () => undefined,
    reloadFeatureFlags: () => {},
    group: () => {},
    people: { set: () => {}, set_once: () => {} },
  };
}

export default (function (): null {
  if (!ExecutionEnvironment.canUseDOM) {
    return null;
  }

  let originalPostHogInit: any = null;

  function setupPostHogOverride(): void {
    // Wait for runtime config to load
    if (!window.POSTHOG_CONFIG) {
      setTimeout(setupPostHogOverride, 50);
      return;
    }

    const config = window.POSTHOG_CONFIG;
    const hasValidKey = isValidPostHogKey(config.apiKey);

    // If disabled or invalid, block PostHog
    if (!config.enabled || !hasValidKey) {
      console.log('PostHog disabled:', {
        enabled: config.enabled,
        hasValidKey,
        keyProvided: !!config.apiKey,
      });

      // Replace posthog with stub
      window.posthog = createPostHogStub();
      
      // Prevent it from being overwritten
      Object.defineProperty(window, 'posthog', {
        get: () => createPostHogStub(),
        set: () => {
          console.log('PostHog initialization blocked');
        },
        configurable: true,
      });

      return;
    }

    // Valid config - wait for posthog-docusaurus to create window.posthog
    function waitForPostHog() {
      if (!window.posthog || typeof window.posthog.init !== 'function') {
        setTimeout(waitForPostHog, 50);
        return;
      }

      // Intercept init only once
      if (!originalPostHogInit) {
        originalPostHogInit = window.posthog.init;

        window.posthog.init = function (apiKey: string, options: any = {}) {
          console.log('🔄 PostHog init intercepted');
          console.log('  Build-time key:', apiKey.substring(0, 15) + '...');
          console.log('  Runtime key:', config.apiKey.substring(0, 15) + '...');
          console.log('  Using apiHost:', config.apiHost);

          // Override with runtime config
          const runtimeOptions = {
            ...options,
            api_host: config.apiHost,
          };

          // Call original init with runtime values
          const result = originalPostHogInit.call(
            this,
            config.apiKey,
            runtimeOptions
          );

          console.log('✅ PostHog initialized with runtime config');
          return result;
        };

        console.log('✅ PostHog override ready');
      }
    }

    waitForPostHog();
  }

  setupPostHogOverride();

  return null;
})();
