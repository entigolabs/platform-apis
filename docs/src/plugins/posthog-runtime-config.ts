import type { Plugin, LoadContext } from '@docusaurus/types';

export default function posthogRuntimeConfigPlugin(
  context: LoadContext,
  options: any
): Plugin {
  return {
    name: 'posthog-runtime-config',

    injectHtmlTags() {
      return {
        headTags: [
          // Only load the runtime config
          // React components will handle initialization
          {
            tagName: 'script',
            attributes: {
              src: '/posthog-config.js',
            },
          },
        ],
      };
    },

    // Load the client module
    getClientModules() {
      return [require.resolve('./posthog-client')];
    },
  };
}