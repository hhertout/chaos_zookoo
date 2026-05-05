import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'chaos_zookoo',
  tagline: 'RBAC-native Kubernetes chaos engineering, driven from the API server',
  favicon: 'img/zookoo_tight.png',

  future: {
    v4: true,
  },

  url: 'https://hhertout.github.io',
  baseUrl: '/chaos_zookoo/',

  organizationName: 'hhertout',
  projectName: 'chaos_zookoo',

  onBrokenLinks: 'throw',

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl:
            'https://github.com/hhertout/chaos_zookoo/tree/main/documentation/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/chaos-zookoo-social-card.jpg',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'zookoo',
      logo: {
        alt: 'zookoo logo',
        src: 'img/zookoo_tight.png',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docsSidebar',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/hhertout/chaos_zookoo',
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
            {label: 'Introduction', to: '/docs/intro'},
            {label: 'Getting started', to: '/docs/getting-started/installation'},
            {label: 'Modules', to: '/docs/modules/overview'},
            {label: 'Middlewares', to: '/docs/middlewares/overview'},
          ],
        },
        {
          title: 'Project',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/hhertout/chaos_zookoo',
            },
            {
              label: 'Issues',
              href: 'https://github.com/hhertout/chaos_zookoo/issues',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} chaos_zookoo contributors. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.vsDark,
      additionalLanguages: ['bash', 'yaml', 'go', 'toml'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
