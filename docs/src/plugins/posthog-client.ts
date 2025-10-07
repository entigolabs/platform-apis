import posthog from 'posthog-js';
import ExecutionEnvironment from '@docusaurus/ExecutionEnvironment';

interface PostHogRuntimeConfig {
  apiKey: string;
  apiHost: string;
  enabled: boolean;
}

declare global {
  interface Window {
    POSTHOG_CONFIG?: PostHogRuntimeConfig;
    posthog?: typeof posthog;
  }
}

function isValidPostHogKey(key: string): boolean {
  return key && key.startsWith('phc_') && key.length >= 20;
}

// Initialize PostHog once
if (ExecutionEnvironment.canUseDOM && typeof window.posthog == 'undefined') {
  function initializePostHog() {
    if (!window.POSTHOG_CONFIG) {
      setTimeout(initializePostHog, 50);
      return;
    }

    const config = window.POSTHOG_CONFIG;
    const hasValidKey = isValidPostHogKey(config.apiKey);

    if (!config.enabled || !hasValidKey) {
      console.log('PostHog disabled:', {
        enabled: config.enabled,
        hasValidKey,
        keyProvided: !!config.apiKey,
      });
      return;
    }

    try {
      posthog.init(config.apiKey, {
        api_host: config.apiHost,
        ui_host: 'https://eu.posthog.com',

        // Capture settings
        capture_pageview: false,
        capture_pageleave: true,
        autocapture: true,
        
        // Session recording
        disable_session_recording: false,
        session_recording: {
          maskAllInputs: true,
          maskTextSelector: '.sensitive',
        },

        persistence: 'localStorage+cookie',

        // Debug
        debug: false,

        loaded: (ph) => {
          
          window.posthog = ph;

          // Send initial event
          ph.capture('posthog_initialized', {
            page: window.location.pathname,
          });
        },
      });

    } catch (error) {
      console.error('Failed to initialize PostHog:', error);
    }
  }

  initializePostHog();
}

export default (function () {
  if (!ExecutionEnvironment.canUseDOM) {
    return null;
  }

  return {
    onRouteUpdate({ location, previousLocation }) {
      if (location.pathname != previousLocation?.pathname) {
        if (typeof window !== 'undefined' && typeof window.posthog !== 'undefined') {
          window.posthog.capture('$pageview');
        }
      }
    },
  };
})();
