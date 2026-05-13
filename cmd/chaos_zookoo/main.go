package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hhertout/chaos_zookoo/internal/config"
	"github.com/hhertout/chaos_zookoo/internal/orchestrator"
	"github.com/hhertout/chaos_zookoo/pkg/gorillakill"
	"github.com/hhertout/chaos_zookoo/pkg/killing"
	"github.com/hhertout/chaos_zookoo/pkg/loadkit"
	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"github.com/hhertout/chaos_zookoo/pkg/module"
	"github.com/hhertout/chaos_zookoo/pkg/nodedrain"
	"github.com/hhertout/chaos_zookoo/pkg/rollout"
	"github.com/hhertout/chaos_zookoo/pkg/testkit"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
)

var builders = map[string]module.Builder{
	"Killing":     killing.Build,
	"Rollout":     rollout.Build,
	"GorillaKill": gorillakill.Build,
	"NodeDrain":   nodedrain.Build,
}

func main() {
	logCfg := zap.NewProductionConfig()
	logCfg.EncoderConfig.EncodeTime = zapcore.RFC3339TimeEncoder
	logger, err := logCfg.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync() //nolint:errcheck
	zap.ReplaceGlobals(logger)

	if err := run(); err != nil {
		zap.L().Error("fatal", zap.Error(err))
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("loading .env: %w", err)
	}

	configPath := flag.String("config", "", "path to config directory or single config file")
	flag.StringVar(configPath, "C", "", "shorthand for --config")
	flag.Parse()

	if *configPath == "" {
		*configPath = os.Getenv("CHAOS_CONFIG_DIR")
	}
	if *configPath == "" {
		*configPath = "configs"
	}

	entries, err := config.LoadEntries(*configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	zap.L().Info("loaded config entries", zap.Int("kinds", len(entries)))

	k8sCfg, err := buildK8sConfig()
	if err != nil {
		return fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	tr := testkit.BuildRunner(os.Getenv)
	loadSup := loadkit.NewSupervisor()

	orch := orchestrator.New()
	if err := registerModules(orch, client, tr, loadSup, entries); err != nil {
		return fmt.Errorf("registering modules: %w", err)
	}

	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9090"
	}
	metricsSrv := metrics.NewServer(metricsAddr)
	metricsSrv.Start()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	orch.Start(ctx)
	<-ctx.Done()

	zap.L().Info("shutting down...")
	orch.Stop()
	loadSup.Stop()
	tr.Stop()
	metricsSrv.Shutdown(context.Background())

	return nil
}

func registerModules(orch *orchestrator.Orchestrator, client kubernetes.Interface, runner *testkit.Runner, loadSup *loadkit.Supervisor, entries config.Entries) error {
	seen := make(map[string]struct{})
	for kind, docs := range entries {
		build, ok := builders[kind]
		if !ok {
			return fmt.Errorf("unknown module kind: %s", kind)
		}

		for _, data := range docs {
			m, err := build(client, data)
			if err != nil {
				return err
			}
			if _, dup := seen[m.Name()]; dup {
				return fmt.Errorf("duplicate module name %q: each module must have a unique name", m.Name())
			}
			seen[m.Name()] = struct{}{}

			cc, err := config.ParseCrossCutting(data, m.Schedule().Interval)
			if err != nil {
				return fmt.Errorf("module %q: %w", m.Name(), err)
			}

			testMw, err := testkit.NewMiddleware(runner, cc.Testing)
			if err != nil {
				return fmt.Errorf("module %q: %w", m.Name(), err)
			}

			loadMw := loadkit.NewMiddleware(loadSup, cc.Load)

			orch.Register(testMw(loadMw(m)))
		}
	}
	return nil
}
