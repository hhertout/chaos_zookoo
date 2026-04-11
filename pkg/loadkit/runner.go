package loadkit

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hhertout/chaos_zookoo/pkg/metrics"
	"go.uber.org/zap"
)

const perRequestTimeout = 30 * time.Second

// Runner executes a single load burst. Safe to call Run repeatedly.
type Runner struct {
	name       string
	spec       *Spec
	lastErrLog atomic.Int64 // unix nanoseconds of last 4xx/5xx log
}

// NewRunner builds a runner from a module name and a validated spec.
func NewRunner(name string, spec *Spec) *Runner {
	return &Runner{name: name, spec: spec}
}

// Run fires spec.Vus parallel workers for spec.Duration.
func (r *Runner) Run(ctx context.Context) error {
	if r == nil || r.spec == nil {
		return nil
	}

	burstCtx, cancel := context.WithTimeout(ctx, r.spec.duration)
	defer cancel()

	method := r.spec.Requests.Method
	url := r.spec.Requests.URL
	metrics.ChaosLoadingHttpActive.WithLabelValues(r.name, method, url).Set(1)
	defer metrics.ChaosLoadingHttpActive.WithLabelValues(r.name, method, url).Set(0)

	zap.L().Info("load burst starting",
		zap.String("name", r.name),
		zap.Int("vus", r.spec.Vus),
		zap.Duration("duration", r.spec.duration),
	)

	transport := http.DefaultTransport
	if r.spec.SkipTLSVerify {
		transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	client := &http.Client{Timeout: perRequestTimeout, Transport: transport}

	var wg sync.WaitGroup
	for i := 0; i < r.spec.Vus; i++ {
		wg.Go(func() { r.vu(burstCtx, client) })
	}
	wg.Wait()

	zap.L().Debug("load burst finished", zap.String("name", r.name))
	return nil
}

func (r *Runner) vu(ctx context.Context, client *http.Client) {
	ticker := time.NewTicker(r.spec.reqInterval)
	defer ticker.Stop()

	r.fire(ctx, client)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.fire(ctx, client)
		}
	}
}

func (r *Runner) fire(ctx context.Context, client *http.Client) {
	method := r.spec.Requests.Method
	url := r.spec.Requests.URL

	var body io.Reader
	if r.spec.Requests.Body != "" {
		body = strings.NewReader(r.spec.Requests.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		metrics.ChaosLoadRequestsTotal.WithLabelValues(r.name, method, url, "error").Inc()
		return
	}
	if r.spec.Requests.ContentType != "" {
		req.Header.Set("Content-Type", r.spec.Requests.ContentType)
	}

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start).Seconds()
	metrics.ChaosLoadRequestDuration.WithLabelValues(r.name, method, url).Observe(elapsed)

	if err != nil {
		now := time.Now().UnixNano()
		last := r.lastErrLog.Load()
		if now-last >= r.spec.reqInterval.Nanoseconds() && r.lastErrLog.CompareAndSwap(last, now) {
			zap.L().Error("load request failed", zap.String("name", r.name), zap.Error(err))
		}

		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			zap.L().Error("load request failed", zap.String("name", r.name), zap.Error(err))
		}
		if errors.Is(err, context.Canceled) {
			metrics.ChaosLoadRequestsTotal.WithLabelValues(r.name, method, url, "canceled").Inc()
		} else if errors.Is(err, context.DeadlineExceeded) {
			metrics.ChaosLoadRequestsTotal.WithLabelValues(r.name, method, url, "timeout").Inc()
		} else {
			metrics.ChaosLoadRequestsTotal.WithLabelValues(r.name, method, url, "unknown_error").Inc()
		}

		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	class := statusClass(resp.StatusCode)
	if resp.StatusCode >= 400 {
		now := time.Now().UnixNano()
		last := r.lastErrLog.Load()
		if now-last >= r.spec.reqInterval.Nanoseconds() && r.lastErrLog.CompareAndSwap(last, now) {
			zap.L().Warn("load request non-2xx",
				zap.String("name", r.name),
				zap.String("url", url),
				zap.String("method", method),
				zap.Int("status", resp.StatusCode),
			)
		}
	}
	metrics.ChaosLoadRequestsTotal.WithLabelValues(r.name, method, url, class).Inc()
}

func statusClass(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	case code >= 100 && code < 200:
		return "1xx"
	default:
		return strconv.Itoa(code)
	}
}
