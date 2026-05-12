import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting started',
      collapsed: false,
      items: [
        'getting-started/installation',
        'getting-started/configuration',
        'getting-started/running-locally',
        'getting-started/helm',
      ],
    },
    {
      type: 'category',
      label: 'Modules',
      collapsed: false,
      items: [
        'modules/overview',
        'modules/killing',
        'modules/gorillakill',
        'modules/rollout',
      ],
    },
    {
      type: 'category',
      label: 'Middlewares',
      collapsed: false,
      items: [
        'middlewares/overview',
        'middlewares/loadkit',
        'middlewares/testkit',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'concepts/matchers',
        'concepts/scheduling',
        'concepts/metrics',
      ],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: [
        'deployment/helm',
        'deployment/rbac',
      ],
    },
    {
      type: 'category',
      label: 'Development',
      items: [
        'development/architecture',
        'development/adding-a-module',
      ],
    },
  ],
};

export default sidebars;
