// Package metrics exposes the Prometheus metrics produced by chaos_zookoo.
package metrics

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Label name constants used across metric vectors.
const (
	labelName   = "name"
	labelKind   = "kind"
	labelMethod = "method"
)

// ChaosTestSuccess reports the outcome of the last chaos test for a module.
var ChaosTestSuccess = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "chaos_test_success",
		Help: "Outcome of the last chaos test: 1 = pass, 0 = fail.",
	},
	[]string{labelName},
)

// ChaosLoadingHttpActive is 1 while a load burst is firing, 0 otherwise.
var ChaosLoadingHttpActive = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "chaos_loading_http_active",
		Help: "Whether the load test is active (1) or not.",
	},
	[]string{labelName, labelMethod, "url"},
)

// ChaosLoadRequestsTotal counts every HTTP request issued by a load burst.
var ChaosLoadRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "chaos_load_requests_total",
		Help: "Total HTTP requests fired by a load burst.",
	},
	[]string{labelName, labelMethod, "url", "status"},
)

// ChaosLoadRequestDuration observes wall-clock latency of each load request.
var ChaosLoadRequestDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "chaos_load_request_duration_seconds",
		Help:    "Latency of HTTP requests fired by a load burst.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{labelName, labelMethod, "url"},
)

// ── Module observability ─────────────────────────────────────────────────────

// ChaosModuleInfo is a static gauge (always 1) carrying module metadata as
// labels. Use it as a join key in Grafana to enrich other metrics with kind,
// namespace, and schedule information.
var ChaosModuleInfo = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "chaos_module_info",
		Help: "Static gauge (value=1) exposing module metadata as labels.",
	},
	[]string{labelName, labelKind, "namespace", "schedule_type", "schedule_value"},
)

// ChaosModuleRunsTotal counts module executions labelled by outcome.
var ChaosModuleRunsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "chaos_module_runs_total",
		Help: "Total number of module executions, by status (success|error).",
	},
	[]string{labelName, labelKind, "namespace", "status"},
)

// ChaosModuleLastRunTimestamp is the Unix timestamp of the last module execution.
var ChaosModuleLastRunTimestamp = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "chaos_module_last_run_timestamp",
		Help: "Unix timestamp of the last module execution.",
	},
	[]string{labelName, labelKind, "namespace"},
)

// ChaosModuleRunDuration observes how long each module Run() takes.
var ChaosModuleRunDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "chaos_module_run_duration_seconds",
		Help:    "Wall-clock duration of each module Run() call.",
		Buckets: prometheus.DefBuckets,
	},
	[]string{labelName, labelKind, "namespace"},
)

// ChaosPodsAffectedTotal counts pods killed or restarted by a module.
var ChaosPodsAffectedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "chaos_pods_affected_total",
		Help: "Total pods killed or restarted by a chaos module.",
	},
	[]string{labelName, labelKind, "namespace"},
)

func init() {
	prometheus.MustRegister(
		ChaosTestSuccess,
		ChaosLoadingHttpActive,
		ChaosLoadRequestsTotal,
		ChaosLoadRequestDuration,
		ChaosModuleInfo,
		ChaosModuleRunsTotal,
		ChaosModuleLastRunTimestamp,
		ChaosModuleRunDuration,
		ChaosPodsAffectedTotal,
	)
}

// Server runs the /metrics HTTP endpoint.
type Server struct {
	http *http.Server
}

// NewServer builds a /metrics server bound to addr.
func NewServer(addr string) *Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &Server{
		http: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

// Start launches the HTTP server in its own goroutine.
func (s *Server) Start() {
	go func() {
		zap.L().Info("metrics server listening", zap.String("addr", s.http.Addr))
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zap.L().Error("metrics server stopped with error", zap.Error(err))
		}
	}()
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) {
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		zap.L().Warn("metrics server shutdown error", zap.Error(err))
	}
}
