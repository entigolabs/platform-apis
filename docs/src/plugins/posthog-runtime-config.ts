import type { Plugin, LoadContext } from '@docusaurus/types';

export default function posthogRuntimeConfigPlugin(
  context: LoadContext,
  options: any
): Plugin {
  return {
    name: 'posthog-runtime-config',

    // Inject the runtime config script in the HTML head
    injectHtmlTags() {
      return {
        headTags: [
          {
            tagName: 'script',
            attributes: {
              src: '/posthog-config.js',
            },
          },
        ],
      };
    },

    // Load client-side override module
    getClientModules() {
      return [require.resolve('./posthog-init')];
    },

    // Expose default/placeholder config for other plugins to use
    async contentLoaded({ actions }) {
      const { setGlobalData } = actions;
      
      // Set placeholder config for build time
      // Will be overridden at runtime by /posthog-config.js
      setGlobalData({
        posthog: {
          apiKey: 'phc_build_time_placeholder',
          apiHost: '/ingest',
          enabled: false,
        },
      });
    },
  };
}