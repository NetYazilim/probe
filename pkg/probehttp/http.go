package probehttp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/netyazilim/probe"
)

// DefaultTimeout bounds a single request. It matches the TLS probe rather than
// the ping and TCP ones because an attempt here carries the same work: with
// keep-alives off, every request resolves the name, dials, and over HTTPS
// completes the handshake before the response is even asked for.
const DefaultTimeout = 5 * time.Second

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

// newTransport returns the transport every probe runs on.
//
// It is cloned from http.DefaultTransport so the probe keeps proxy support,
// HTTP/2 negotiation and the standard dial and handshake timeouts, but with
// connection reuse switched off.
//
// Keep-alives have to go because they hide outages. http.DefaultTransport is a
// process-wide pool, so a client built with the zero Transport shares idle
// connections with every other request in the program, across calls. A probe
// that runs more often than the idle timeout then answers over a connection
// that is already open: it resolves no name, dials nothing and repeats no TLS
// handshake, and so keeps reporting success while DNS, routing or the
// certificate are broken. Each attempt has to walk the whole path.
func newTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableKeepAlives = true
	return t
}

// RunContext performs an HTTP/HTTPS probe against url, honouring ctx for
// cancellation both between and during requests.
//
// Every attempt opens its own connection, so the measured duration covers name
// resolution, the TCP dial and, for HTTPS, the TLS handshake.
func RunContext(ctx context.Context, url string, opts probe.Options) Result {
	opts = opts.WithDefaults(DefaultTimeout)
	if err := opts.Validate(); err != nil {
		return Result{Summary: probe.Summary{Error: err}, URL: url}
	}

	result := Result{URL: url}
	client := &http.Client{Timeout: opts.Timeout, Transport: newTransport()}
	defer client.CloseIdleConnections()

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

		// Read the body to the end before closing so the server sees the
		// response finish rather than a reset. The connection closes either
		// way, because keep-alives are off.
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
