---
title: loadkit
sidebar_position: 2
---

# loadkit

`loadkit` fires a synthetic HTTP burst in parallel with the chaos action
it wraps. Useful to validate behavior **under concurrent load** — e.g.
"does the service still respond 2xx while a pod is being evicted?".

## Config block

```yaml
load:
  vus: 10                    # required — number of parallel virtual users
  duration: 30s              # required — how long the burst lasts
  skipTlsVerify: false       # optional — disable TLS certificate verification (default false)
  requests:
    method: GET              # required — GET or POST
    url: https://my-app.example.com/healthz   # required
    interval: 100ms          # optional — per-VU delay between requests (default 100ms)
    contentType: application/json  # optional — sets Content-Type header (defaults to application/json when body is set)
    body: '{"key":"value"}'  # optional — request body for POST
```

## Field reference

| Field                       | Type       | Default           | Notes                                                                       |
| --------------------------- | ---------- | ----------------- | --------------------------------------------------------------------------- |
| `load.vus`                  | integer    | —                 | Required, `> 0`. Number of parallel goroutines firing requests.             |
| `load.duration`             | duration   | —                 | Required, `> 0`. Must be `< scenario.interval` if the module is periodic.   |
| `load.skipTlsVerify`        | boolean    | `false`           | Disables TLS certificate verification. Use for self-signed certs or internal CAs. |
| `load.requests.method`      | enum       | —                 | Required. `GET` or `POST`.                                                  |
| `load.requests.url`         | URL        | —                 | Required. Must start with `http://` or `https://`.                          |
| `load.requests.interval`    | duration   | `100ms`           | Per-VU sleep between two requests. Must be `> 0`.                           |
| `load.requests.body`        | string     | —                 | Optional request body. Typically used with `POST`.                          |
| `load.requests.contentType` | string     | `application/json`| Content-Type header. Auto-set to `application/json` when `body` is present. |

## Behavior

When a wrapped module's `Run` is called, the middleware:

1. Immediately spawns `vus` goroutines bound to a context with a
   `duration` timeout.
2. Each goroutine loops: fire a request, sleep `requests.interval`,
   repeat — until the context expires.
3. The chaos action runs **in parallel** in the main goroutine.
4. The `loadkit.Supervisor` holds a `WaitGroup` so the burst is drained
   at shutdown.

Body handling is deliberately minimal: the response body is `io.Copy`-ed
to `io.Discard` and closed. No retries, no assertions — this is a traffic
generator, not a test client. Use [testkit](./testkit.md) if you want to
assert on the outcome.

## Metrics

Every request is tracked via Prometheus:

| Metric                               | Type       | Labels                            |
| ------------------------------------ | ---------- | --------------------------------- |
| `chaos_loading_http_active`          | gauge      | `name`, `method`, `url`           |
| `chaos_load_requests_total`          | counter    | `name`, `method`, `url`, `status` |
| `chaos_load_request_duration_seconds`| histogram  | `name`, `method`, `url`           |

`status` is bucketed into `2xx` / `3xx` / `4xx` / `5xx` / `1xx` /
`error` (`error` covers network failures and context cancellation).

The `name` label is the module name — not the load config — so metrics
join naturally with `chaos_test_success`.

## Limitations

- One endpoint per module. Multi-endpoint load patterns require multiple
  modules.
- No custom headers / auth (yet). The generator targets public or
  token-in-URL endpoints.

These are deliberate tradeoffs — `loadkit` is meant to generate
**realistic-enough** load during chaos, not to replace k6 / Gatling.
