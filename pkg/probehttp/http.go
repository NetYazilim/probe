package probehttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/netyazilim/probe"
)

// DefaultTimeout bounds a single request.
const DefaultTimeout = 1 * time.Second

// Result holds HTTP probe result
type Result struct {
	probe.Summary

	URL string

	// StatusCode is the status of the last response that was received, whether
	// or not it counted as a success.
	StatusCode int
}

// Run performs an HTTP/HTTPS probe against url, retrying up to maxAttempts
// times with timeout per attempt.
//
// It is a thin wrapper over RunContext kept for callers written against the
// original API. Zero values are replaced by the package defaults.
func Run(url string, maxAttempts int, timeout time.Duration) Result {
	return RunContext(context.Background(), url, probe.Options{
		MaxAttempts: maxAttempts,
		Timeout:     timeout,
	})
}

// RunContext performs an HTTP/HTTPS probe against url, honouring ctx for
// cancellation both between and during requests.
func RunContext(ctx context.Context, url string, opts probe.Options) Result {
	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{Summary: probe.Summary{Error: err}, URL: url}
	}

	result := Result{URL: url}
	client := &http.Client{Timeout: opts.Timeout}

	stats := probe.Repeat(ctx, opts, func(ctx context.Context) (time.Duration, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return 0, fmt.Errorf("invalid request: %v", err)
		}

		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return time.Since(start), fmt.Errorf("request failed: %v", err)
		}

		// Drain before closing so the connection can be reused; without this
		// every attempt opens a fresh one.
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		elapsed := time.Since(start)

		result.StatusCode = resp.StatusCode
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return elapsed, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return elapsed, nil
	})

	result.Summary = stats.Summarize(opts.SuccessThreshold)
	return result
}
