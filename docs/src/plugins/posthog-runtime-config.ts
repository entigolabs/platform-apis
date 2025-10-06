import type { Plugin } from '@docusaurus/types';

export default function posthogRuntimeConfigPlugin(): Plugin {
  return {
    name: 'posthog-runtime-config',

    injectHtmlTags() {
      return {
        headTags: [
          // Inject the runtime config before PostHog initializes
          {
            tagName: 'script',
            attributes: {
              src: '/posthog-config.js',
            },
          },
        ],
      };
    },

    // This runs in the browser
    getClientModules() {
      return [require.resolve('./posthog-init')];
    },
  };
}