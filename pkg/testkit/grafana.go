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

// Querier evaluates a test spec and returns a single scalar value.
type Querier interface {
	Query(ctx context.Context, details Details) (float64, error)
}

// GrafanaClient queries Prometheus datasources through the Grafana datasource proxy.
type GrafanaClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewGrafanaClient builds a client targeting the given Grafana base URL.
// token is sent as a bearer credential if non-empty. The caller-supplied
// httpClient is used for all outgoing requests; pass nil for a sensible default.
func NewGrafanaClient(baseURL, token string, httpClient *http.Client) *GrafanaClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &GrafanaClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    httpClient,
	}
}

// Query runs a Prometheus query_range through Grafana's datasource proxy and
// returns the last numeric value of the first series.
func (g *GrafanaClient) Query(ctx context.Context, details Details) (float64, error) {
	if details.DatasourceKind != DatasourcePrometheus {
		return 0, fmt.Errorf("grafana client: unsupported datasourceKind %q", details.DatasourceKind)
	}

	end := time.Now()
	start := end.Add(-details.timeWindow)
	step := int64(details.timeWindow.Seconds())
	if step <= 0 {
		step = 1
	}

	endpoint := fmt.Sprintf("%s/api/datasources/proxy/uid/%s/api/v1/query_range",
		g.baseURL, url.PathEscape(details.DatasourceID))

	q := url.Values{}
	q.Set("query", details.Query)
	q.Set("start", strconv.FormatInt(start.Unix(), 10))
	q.Set("end", strconv.FormatInt(end.Unix(), 10))
	q.Set("step", strconv.FormatInt(step, 10))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("building grafana request: %w", err)
	}
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := g.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("calling grafana: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("reading grafana response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("grafana returned status %d: %s", resp.StatusCode, string(body))
	}

	var parsed promQueryRangeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("decoding grafana response: %w", err)
	}
	if parsed.Status != "success" {
		return 0, fmt.Errorf("prometheus query failed: status=%s errorType=%s error=%s", parsed.Status, parsed.ErrorType, parsed.Error)
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

type promQueryRangeResponse struct {
	Status    string             `json:"status"`
	ErrorType string             `json:"errorType,omitempty"`
	Error     string             `json:"error,omitempty"`
	Data      promQueryRangeData `json:"data"`
}

type promQueryRangeData struct {
	ResultType string       `json:"resultType"`
	Result     []promSeries `json:"result"`
}

type promSeries struct {
	Metric map[string]string `json:"metric"`
	Values []promSample      `json:"values"`
}

type promSample struct {
	Timestamp float64
	Value     string
}

func (s *promSample) UnmarshalJSON(data []byte) error {
	var raw [2]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("sample tuple: %w", err)
	}
	if err := json.Unmarshal(raw[0], &s.Timestamp); err != nil {
		return fmt.Errorf("sample timestamp: %w", err)
	}
	if err := json.Unmarshal(raw[1], &s.Value); err != nil {
		return fmt.Errorf("sample value: %w", err)
	}
	return nil
}
