import type {ReactNode} from 'react';
import clsx from 'clsx';
import Heading from '@theme/Heading';
import styles from './styles.module.css';

type FeatureItem = {
  title: string;
  icon: string;
  description: ReactNode;
};

const FeatureList: FeatureItem[] = [
  {
    title: 'RBAC-native',
    icon: '🔒',
    description: (
      <>
        No privileged pods, no host namespaces, no webhooks. Every disruption
        is a plain Kubernetes API call authenticated as a{' '}
        <code>ServiceAccount</code>. Your cluster RBAC <em>is</em> the
        security model.
      </>
    ),
  },
  {
    title: 'YAML-driven',
    icon: '📝',
    description: (
      <>
        Declare scenarios in YAML just like Chaos Mesh CRDs — but consumed
        locally, not applied to the cluster. Mix multiple kinds and
        namespaces in a single file or a directory.
      </>
    ),
  },
  {
    title: 'Composable middlewares',
    icon: '🧩',
    description: (
      <>
        Wrap any module with synthetic HTTP load (<code>loadkit</code>) or
        post-run observability assertions (<code>testkit</code>). Modules
        stay unaware; cross-cutting concerns stay orthogonal.
      </>
    ),
  },
];

function Feature({title, icon, description}: FeatureItem) {
  return (
    <div className={clsx('col col--4')}>
      <div className="text--center">
        <div className={styles.featureIcon} aria-hidden="true">
          {icon}
        </div>
      </div>
      <div className="text--center padding-horiz--md">
        <Heading as="h3">{title}</Heading>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function HomepageFeatures(): ReactNode {
  return (
    <section className={styles.features}>
      <div className="container">
        <div className="row">
          {FeatureList.map((props, idx) => (
            <Feature key={idx} {...props} />
          ))}
        </div>
      </div>
    </section>
  );
}
