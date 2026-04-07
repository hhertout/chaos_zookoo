package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/hhertout/chaos_zookoo/pkg/module"
	"github.com/hhertout/chaos_zookoo/pkg/module/killing"
	"github.com/hhertout/chaos_zookoo/pkg/module/rollout"
	"github.com/hhertout/chaos_zookoo/pkg/orchestrator"
	"k8s.io/client-go/kubernetes"
)

const (
	kindKilling = "Killing"
	kindRollout = "Rollout"
)

func main() {
	configPath := flag.String("config", "", "path to config directory or single config file")
	flag.Parse()

	if *configPath == "" {
		*configPath = os.Getenv("CHAOS_CONFIG_DIR")
	}
	if *configPath == "" {
		*configPath = "configs"
	}

	entries, err := module.LoadEntries(*configPath)
	if err != nil {
		slog.Error("failed to load config entries", "error", err)
		os.Exit(1)
	}
	slog.Info("loaded config entries", "kinds", len(entries))

	k8sConfig, err := buildK8sConfig()
	if err != nil {
		slog.Error("failed to build kubernetes config", "error", err)
		os.Exit(1)
	}

	client, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		slog.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}

	orch := orchestrator.New()

	if err := registerModules(orch, client, entries); err != nil {
		slog.Error("failed to register modules", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orch.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	slog.Info("shutting down...")
	cancel()
	orch.Stop()
}

func registerModules(orch *orchestrator.Orchestrator, client kubernetes.Interface, entries module.Entries) error {
	for kind, files := range entries {
		for _, data := range files {
			switch kind {
			case kindKilling:
				cfg, err := killing.ParseConfig(data)
				if err != nil {
					return fmt.Errorf("invalid killing config: %w", err)
				}
				orch.Register(killing.New(client, cfg))

			case kindRollout:
				cfg, err := rollout.ParseConfig(data)
				if err != nil {
					return fmt.Errorf("invalid rollout config: %w", err)
				}
				orch.Register(rollout.New(client, cfg))

			default:
				return fmt.Errorf("unknown experiment kind: %s", kind)
			}
		}
	}
	return nil
}
