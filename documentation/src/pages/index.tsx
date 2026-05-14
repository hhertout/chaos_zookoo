import type {ReactNode, RefObject} from 'react';
import {useRef, useState, useEffect} from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import {useColorMode} from '@docusaurus/theme-common';
import {Highlight, themes} from 'prism-react-renderer';
import styles from './index.module.css';

function useInView(threshold = 0.12) {
  const ref = useRef<HTMLElement>(null);
  const [inView, setInView] = useState(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new IntersectionObserver(
      ([e]) => { if (e.isIntersecting) { setInView(true); obs.disconnect(); } },
      {threshold, rootMargin: '0px 0px -48px 0px'},
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, [threshold]);
  return [ref, inView] as const;
}

function rv(...extra: (string | false | undefined)[]) {
  return [styles.reveal, ...extra.filter(Boolean)].join(' ');
}

const FEATURES = [
  {
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
        <path d="M9 12l2 2 4-4" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
      </svg>
    ),
    title: 'RBAC-native security',
    description:
      'No cluster-admin required. The ServiceAccount RBAC is the security model — grant exactly the chaos permissions you intend, nothing more.',
  },
  {
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <rect x="2" y="3" width="20" height="14" rx="2" stroke="currentColor" strokeWidth="2"/>
        <path d="M8 21h8M12 17v4" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        <path d="M7 8h.01M11 8h6" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        <path d="M7 12h.01M11 12h6" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
      </svg>
    ),
    title: 'API-server only',
    description:
      'Every disruption is a regular API call — EvictV1, Pods.Delete, Deployments.Patch. No privileged nodes, no DaemonSets, no sidecars.',
  },
  {
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
        <polyline points="14 2 14 8 20 8" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
        <line x1="16" y1="13" x2="8" y2="13" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        <line x1="16" y1="17" x2="8" y2="17" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
      </svg>
    ),
    title: 'YAML-driven config',
    description:
      'Declare scenarios in familiar YAML. Each document maps to a module — Killing, GorillaKill, Rollout — with its own schedule and selectors.',
  },
  {
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="2"/>
        <path d="M19.07 4.93a10 10 0 010 14.14M4.93 4.93a10 10 0 000 14.14" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
        <path d="M15.54 8.46a5 5 0 010 7.07M8.46 8.46a5 5 0 000 7.07" stroke="currentColor" strokeWidth="2" strokeLinecap="round"/>
      </svg>
    ),
    title: 'Composable middlewares',
    description:
      'Wrap any module with synthetic HTTP load generation and post-run Prometheus assertions — without touching the module code.',
  },
  {
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" stroke="currentColor" strokeWidth="2"/>
        <path d="M8 12l2.5 2.5L16 9" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
        <path d="M12 6v1M12 17v1M6 12h1M17 12h1" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round"/>
      </svg>
    ),
    title: 'Measurable resilience',
    description:
      'Each passing run is a proof point, not an opinion. Accumulate a versioned track record of recovery — and gate releases on it instead of hope.',
  },
];

const YAML_SRC = `kind: Killing
metadata:
  name: frontend-pod-killer
  namespace: production
scenario:
  interval: 5m
  wait: 30s
  minAvailable: 2
  dryRun: false
  matchers:
    labels:
      app: frontend
      tier: web`;

function YamlCodeBlock(): ReactNode {
  const {colorMode} = useColorMode();
  const theme = colorMode === 'dark' ? themes.vsDark : themes.github;
  return (
    <div className={styles.codeWindow}>
      <div className={styles.codeWindowBar}>
        <span className={styles.dot} style={{background: '#ff5f56'}} />
        <span className={styles.dot} style={{background: '#ffbd2e'}} />
        <span className={styles.dot} style={{background: '#27c93f'}} />
        <span className={styles.codeWindowTitle}>killing.yaml</span>
      </div>
      <Highlight theme={theme} code={YAML_SRC} language="yaml">
        {({style, tokens, getLineProps, getTokenProps}) => (
          <pre className={styles.codeBlock} style={style}>
            {tokens.map((line, i) => (
              <div key={i} {...getLineProps({line})}>
                {line.map((token, key) => (
                  <span key={key} {...getTokenProps({token})} />
                ))}
              </div>
            ))}
          </pre>
        )}
      </Highlight>
    </div>
  );
}

const DELAYS = [styles.d1, styles.d2, styles.d3, styles.d4, styles.d5];

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();

  const [sreRef,      sreVisible]      = useInView();
  const [featuresRef, featuresVisible] = useInView();
  const [ctaRef,      ctaVisible]      = useInView();

  return (
    <Layout title={siteConfig.title} description={siteConfig.tagline}>
      <main className={styles.main}>

        {/* ── Hero ── */}
        <section className={styles.hero}>
          <div className={styles.heroGrid} aria-hidden="true" />
          <div className={styles.heroGlow} aria-hidden="true" />

          <div className={styles.heroContent}>
            <div className={styles.heroBadge}>
              <span className={styles.heroBadgeDot} />
              Build resilience · Kubernetes chaos engineering
            </div>

            <h1 className={styles.heroTitle}>
              Chaos engineering{' '}<br/>
              <span className={styles.heroAccent}>with your RBAC</span>
            </h1>

            <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>

            <div className={styles.heroActions}>
              <Link className={styles.btnPrimary} to="/docs/intro">
                Get started
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M5 12h14M12 5l7 7-7 7" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </Link>
              <Link className={styles.btnSecondary} href="https://github.com/hhertout/chaos_zookoo">
                <svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                  <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z"/>
                </svg>
                GitHub
              </Link>
            </div>
          </div>

          <div className={styles.heroCodeWrap}>
            <YamlCodeBlock />
          </div>
        </section>

        {/* ── SRE positioning ── */}
        <section className={styles.sre}>
          <div className={styles.sreGlow} aria-hidden="true" />
          <div className={styles.sreInner} ref={sreRef as RefObject<HTMLDivElement>}>
            <div className={`${styles.sreHeader} ${rv(sreVisible && styles.visible)}`}>
              <p className={styles.sectionLabel}>Built for SRE teams</p>
              <h2 className={styles.sectionTitle}>
                Rehearse the failure <em>before</em> production does it for you.
              </h2>
              <p className={styles.sectionSubtitle}>
                Every incident you ship to prod is a rehearsal you skipped.
                chaos_zookoo turns crash and recovery scenarios into versioned,
                reproducible YAML — so you can prove your workloads survive
                the fault in staging, on a schedule, with an auditable pass/fail
                signal, <em>long</em> before a pager wakes anyone up.
              </p>
            </div>

            <ol className={styles.sreSteps}>
              <li className={`${styles.sreStep} ${rv(styles.d1, sreVisible && styles.visible)}`}>
                <span className={styles.sreStepNumber}>01</span>
                <h3 className={styles.sreStepTitle}>Describe the failure</h3>
                <p className={styles.sreStepDesc}>
                  Pick a workload, a fault kind (kill, mass kill, restart),
                  a cadence, and a safety floor. One YAML doc per scenario —
                  readable by anyone on the team.
                </p>
              </li>
              <li className={styles.sreConnector} aria-hidden="true" />
              <li className={`${styles.sreStep} ${rv(styles.d2, sreVisible && styles.visible)}`}>
                <span className={styles.sreStepNumber}>02</span>
                <h3 className={styles.sreStepTitle}>Run &amp; observe</h3>
                <p className={styles.sreStepDesc}>
                  Fire synthetic traffic during the disruption, then query
                  Prometheus to assert the SLO held. Results land in your
                  existing Grafana dashboards.
                </p>
              </li>
              <li className={styles.sreConnector} aria-hidden="true" />
              <li className={`${styles.sreStep} ${rv(styles.d3, sreVisible && styles.visible)}`}>
                <span className={styles.sreStepNumber}>03</span>
                <h3 className={styles.sreStepTitle}>Ship with evidence</h3>
                <p className={styles.sreStepDesc}>
                  A green <code className={styles.sreCode}>chaos_test_success</code>{' '}
                  is a reproducible signal that the workload recovers. Gate
                  your release on it — not on hope.
                </p>
              </li>
            </ol>
          </div>
        </section>

        {/* ── Features ── */}
        <section className={styles.features}>
          <div className={styles.featuresInner} ref={featuresRef as RefObject<HTMLDivElement>}>
            <div className={`${styles.sectionHeader} ${rv(featuresVisible && styles.visible)}`}>
              <p className={styles.sectionLabel}>Why chaos_zookoo</p>
              <h2 className={styles.sectionTitle}>Precision chaos, minimal footprint</h2>
              <p className={styles.sectionSubtitle}>
                A single long-running process authenticated as a ServiceAccount. No custom resources, no operator, no privileged components.
              </p>
            </div>
            <div className={styles.featuresGrid}>
              {FEATURES.map((f, i) => (
                <div
                  key={f.title}
                  className={`${styles.featureCard} ${rv(DELAYS[i % DELAYS.length], featuresVisible && styles.visible)}`}
                >
                  <div className={styles.featureIcon}>{f.icon}</div>
                  <h3 className={styles.featureTitle}>{f.title}</h3>
                  <p className={styles.featureDesc}>{f.description}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* ── CTA ── */}
        <section className={styles.cta}>
          <div className={styles.ctaGlow} aria-hidden="true" />
          <div className={styles.ctaInner} ref={ctaRef as RefObject<HTMLDivElement>}>
            <h2 className={`${styles.ctaTitle} ${rv(ctaVisible && styles.visible)}`}>
              Ready to break things safely?
            </h2>
            <p className={`${styles.ctaSubtitle} ${rv(styles.d1, ctaVisible && styles.visible)}`}>
              Follow the installation guide and run your first chaos scenario in minutes.
            </p>
            <div className={`${styles.ctaActions} ${rv(styles.d2, ctaVisible && styles.visible)}`}>
              <Link className={styles.btnPrimary} to="/docs/getting-started/installation">
                Install chaos_zookoo
              </Link>
              <Link className={styles.btnGhost} to="/docs/modules/overview">
                Explore modules
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
                  <path d="M5 12h14M12 5l7 7-7 7" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round"/>
                </svg>
              </Link>
            </div>
          </div>
        </section>

      </main>
    </Layout>
  );
}
