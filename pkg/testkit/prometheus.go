package testkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// PrometheusClient queries the Prometheus HTTP API directly (no Grafana proxy).
type PrometheusClient struct {
	baseURL  string
	token    string
	username string
	password string
	http     *http.Client
}

// NewPrometheusClient builds a client targeting the given Prometheus base URL.
// Bearer token takes precedence over basic auth when both are non-empty.
// Pass nil for httpClient to get a sensible default.
func NewPrometheusClient(baseURL, token, username, password string, httpClient *http.Client) *PrometheusClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &PrometheusClient{
		baseURL:  strings.TrimRight(baseURL, "/"),
		token:    token,
		username: username,
		password: password,
		http:     httpClient,
	}
}

// Query runs a Prometheus query_range and returns the last numeric value of the first series.
func (p *PrometheusClient) Query(ctx context.Context, details Details) (float64, error) {
	end := time.Now()
	start := end.Add(-details.timeWindow)
	step := int64(details.timeWindow.Seconds())
	if step <= 0 {
		step = 1
	}

	q := url.Values{}
	q.Set("query", details.Query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.FormatInt(step, 10))

	endpoint := fmt.Sprintf("%s/api/v1/query_range?%s", p.baseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, fmt.Errorf("building prometheus request: %w", err)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	} else if p.username != "" {
		req.SetBasicAuth(p.username, p.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("calling prometheus: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading prometheus response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("prometheus returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed promQueryRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("decoding prometheus response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query failed: status=%s errorType=%s error=%s",
			parsed.Status, parsed.ErrorType, parsed.Error)
	}
	if len(parsed.Data.Result) == 0 || len(parsed.Data.Result[0].Values) == 0 {
		return 0, fmt.Errorf("prometheus returned no data points")
	}

	samples := parsed.Data.Result[0].Values
	last := samples[len(samples)-1]
	v, err := strconv.ParseFloat(last.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing prometheus value %q: %w", last.Value, err)
	}
	return v, nil
}
