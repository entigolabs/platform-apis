import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

import posthogRuntimeConfigPlugin from './src/plugins/posthog-runtime-config';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Entigo Documentation',
  tagline: '',
  favicon: 'img/entigo-icon.png',

  // Future flags, see https://docusaurus.io/docs/api/docusaurus-config#future
  future: {
    v4: true, // Improve compatibility with the upcoming Docusaurus v4
  },

  // Set the production url of your site here
  url: 'https://docs.entigo.com',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'facebook', // Usually your GitHub org/user name.
  projectName: 'docusaurus', // Usually your repo name.

  onBrokenLinks: 'warn',
  onBrokenAnchors: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: 'docs',
          routeBasePath: 'docs',          
          sidebarPath: './sidebars.ts',
          editUrl:
            'https://github.com/entigolabs/platform-apis/docs',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],
  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      {
        id: 'api',
        path: 'api',
        routeBasePath: 'api',
        sidebarPath: './sidebars.ts',
      },
    ],
    // Custom plugin to inject PostHog runtime config
    posthogRuntimeConfigPlugin,
    
    // Official PostHog plugin with configuration
    [
      'posthog-docusaurus',
      {
        // Placeholder values - will be overridden at runtime
        apiKey: 'phc_build_time_placeholder',
        appUrl: 'https://eu.posthog.com',
        apiHost: '/ingest',
        enableInDevelopment: false,

        // Optional: additional PostHog options
        options: {
          autocapture: true,
          capture_pageview: true,
          capture_pageleave: true,
          disable_session_recording: false,
          session_recording: {
            maskAllInputs: true,
            maskTextSelector: '.sensitive',
          },
        },
      },
    ],
  ],
  
  themeConfig: {
    // Replace with your project's social card
    image: 'img/docusaurus-social-card.jpg',
    metadata: [
      {name: 'edition', content: 'saas,open-source'},
    ],    
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Docs',
      logo: {
        alt: 'Entigo Logo',
        src: 'img/entigo-icon.png',
      },
      items: [
        {
          type: 'doc',
          docId: 'intro',
          position: 'left',
          label: 'Docs',
        },
        {
          type: 'doc',
          docId: 'intro',
          docsPluginId: 'api',
          position: 'left',
          label: 'API',
        },
        {
          href: 'https://github.com/entigolabs',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Tutorial',
              to: '/docs/intro',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Entigo.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
