// Package loadkit drives synthetic HTTP load in parallel with a chaos action.
package loadkit

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultRequestInterval = 100 * time.Millisecond
)

// Spec is the YAML block attached to a module.
type Spec struct {
	Vus           int      `yaml:"vus" json:"vus"`
	Duration      string   `yaml:"duration" json:"duration"`
	Requests      Requests `yaml:"requests" json:"requests"`
	SkipTLSVerify bool     `yaml:"skipTlsVerify" json:"skipTlsVerify"`

	duration    time.Duration
	reqInterval time.Duration
}

// Requests describes the HTTP call each virtual user repeats during the burst.
type Requests struct {
	Method      string `yaml:"method" json:"method"`
	URL         string `yaml:"url" json:"url"`
	Interval    string `yaml:"interval" json:"interval"`
	Body        string `yaml:"body" json:"body"`
	ContentType string `yaml:"contentType" json:"contentType"`
}

// GetDuration returns the resolved burst duration.
func (s *Spec) GetDuration() time.Duration { return s.duration }

// RequestInterval returns the resolved delay between two requests of a single VU.
func (s *Spec) RequestInterval() time.Duration { return s.reqInterval }

// ApplyDefaultsAndValidate fills defaults, parses durations, and checks the
// spec is coherent with the scenario interval. A nil receiver is valid.
func (s *Spec) ApplyDefaultsAndValidate(scenarioInterval time.Duration) error {
	if s == nil {
		return nil
	}

	if s.Vus <= 0 {
		return fmt.Errorf("load.vus must be > 0")
	}

	if s.Duration == "" {
		return fmt.Errorf("load.duration is required")
	}
	d, err := time.ParseDuration(s.Duration)
	if err != nil {
		return fmt.Errorf("invalid load.duration %q: %w", s.Duration, err)
	}
	if d <= 0 {
		return fmt.Errorf("load.duration must be > 0")
	}
	if scenarioInterval > 0 && d >= scenarioInterval {
		return fmt.Errorf("load.duration (%s) must be < scenario.interval (%s)", d, scenarioInterval)
	}
	s.duration = d

	if s.Requests.Interval == "" {
		s.reqInterval = DefaultRequestInterval
	} else {
		ri, err := time.ParseDuration(s.Requests.Interval)
		if err != nil {
			return fmt.Errorf("invalid load.requests.interval %q: %w", s.Requests.Interval, err)
		}
		if ri <= 0 {
			return fmt.Errorf("load.requests.interval must be > 0")
		}
		s.reqInterval = ri
	}

	if s.Requests.Method == "" {
		return fmt.Errorf("load.requests.method is required")
	}
	if s.Requests.Method != http.MethodGet && s.Requests.Method != http.MethodPost {
		return fmt.Errorf("load.requests.method must be GET or POST (got %q)", s.Requests.Method)
	}

	if s.Requests.URL == "" {
		return fmt.Errorf("load.requests.url is required")
	}
	if !strings.HasPrefix(s.Requests.URL, "http://") && !strings.HasPrefix(s.Requests.URL, "https://") {
		return fmt.Errorf("load.requests.url must start with http:// or https://")
	}

	if s.Requests.Body != "" && s.Requests.ContentType == "" {
		s.Requests.ContentType = "application/json"
	}

	return nil
}
